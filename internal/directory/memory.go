package directory

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/auth"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/tenant"
)

// MemoryStore keeps organizations and members in memory. It backs the API
// when no database is configured and the unit tests.
type MemoryStore struct {
	mu   sync.RWMutex
	orgs map[string]*memOrg // by tenant id
}

type memOrg struct {
	name    string
	members map[string]*memMember // by member id
}

type memMember struct {
	Member
	subject string // OIDC subject once the invitation is accepted
}

// NewMemoryStore creates an empty store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{orgs: make(map[string]*memOrg)}
}

func (m *memMember) membership(tenantID, tenantName string) auth.Membership {
	return auth.Membership{
		UserID: m.ID, TenantID: tenantID, TenantName: tenantName,
		Email: m.Email, Name: m.Name, Role: m.Role,
	}
}

// ── auth.Directory ──

func (s *MemoryStore) Resolve(_ context.Context, id auth.Identity) ([]auth.Membership, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []auth.Membership
	for tenantID, org := range s.orgs {
		var match *memMember
		for _, m := range org.members {
			if id.Subject != "" && m.subject == id.Subject {
				match = m // an existing binding always wins over an invitation
				break
			}
			if m.subject == "" && id.EmailVerified && id.Email != "" && m.Email == id.Email {
				match = m
			}
		}
		if match == nil {
			continue
		}
		if match.subject == "" && id.Subject != "" {
			// Accept the invitation.
			now := time.Now().UTC()
			match.subject = id.Subject
			match.LastLogin = &now
			match.Status = "active"
			if match.Name == "" {
				match.Name = id.Name
			}
		}
		result = append(result, match.membership(tenantID, org.name))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].TenantName != result[j].TenantName {
			return result[i].TenantName < result[j].TenantName
		}
		return result[i].TenantID < result[j].TenantID
	})
	return result, nil
}

func (s *MemoryStore) Membership(_ context.Context, tenantID, userID string) (*auth.Membership, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	org, ok := s.orgs[tenantID]
	if !ok {
		return nil, auth.ErrNoMembership
	}
	m, ok := org.members[userID]
	if !ok {
		return nil, auth.ErrNoMembership
	}
	membership := m.membership(tenantID, org.name)
	return &membership, nil
}

func (s *MemoryStore) CreateOrganization(_ context.Context, name string, owner auth.Identity) (*auth.Membership, error) {
	email, ok := NormalizeEmail(owner.Email)
	if !ok {
		return nil, errors.New("directory: the identity has no usable e-mail address")
	}
	now := time.Now().UTC()
	member := &memMember{
		Member: Member{
			ID: tenant.NewUUID(), Email: email, Name: owner.Name, Role: auth.RoleOwner,
			Status: "active", LastLogin: &now, CreatedAt: now,
		},
		subject: owner.Subject,
	}
	tenantID := tenant.NewUUID()

	s.mu.Lock()
	s.orgs[tenantID] = &memOrg{name: name, members: map[string]*memMember{member.ID: member}}
	s.mu.Unlock()

	membership := member.membership(tenantID, name)
	return &membership, nil
}

func (s *MemoryStore) EnsureDevUser(_ context.Context, tenantID, email, name, role string) (*auth.Membership, error) {
	email, ok := NormalizeEmail(email)
	if !ok {
		return nil, errors.New("directory: invalid e-mail address")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	org, ok := s.orgs[tenantID]
	if !ok {
		org = &memOrg{name: devOrganizationName, members: make(map[string]*memMember)}
		s.orgs[tenantID] = org
	}
	now := time.Now().UTC()
	for _, m := range org.members {
		if m.Email == email {
			if role != "" {
				m.Role = role
			}
			m.LastLogin = &now
			m.Status = "active"
			membership := m.membership(tenantID, org.name)
			return &membership, nil
		}
	}
	if role == "" {
		role = auth.RoleAdmin
		if len(org.members) == 0 {
			role = auth.RoleOwner
		}
	}
	m := &memMember{Member: Member{
		ID: tenant.NewUUID(), Email: email, Name: name, Role: role,
		Status: "active", LastLogin: &now, CreatedAt: now,
	}}
	org.members[m.ID] = m
	membership := m.membership(tenantID, org.name)
	return &membership, nil
}

// ── Member management ──

func (s *MemoryStore) ListMembers(_ context.Context, tenantID string) ([]Member, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	members := []Member{}
	if org, ok := s.orgs[tenantID]; ok {
		for _, m := range org.members {
			members = append(members, m.Member)
		}
	}
	sort.Slice(members, func(i, j int) bool {
		if !members[i].CreatedAt.Equal(members[j].CreatedAt) {
			return members[i].CreatedAt.Before(members[j].CreatedAt)
		}
		return members[i].Email < members[j].Email
	})
	return members, nil
}

func (s *MemoryStore) Invite(_ context.Context, tenantID, email, name, role string) (*Member, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	org, ok := s.orgs[tenantID]
	if !ok {
		return nil, ErrNotFound
	}
	for _, m := range org.members {
		if m.Email == email {
			return nil, ErrConflict
		}
	}
	m := &memMember{Member: Member{
		ID: tenant.NewUUID(), Email: email, Name: name, Role: role,
		Status: "invited", CreatedAt: time.Now().UTC(),
	}}
	org.members[m.ID] = m
	member := m.Member
	return &member, nil
}

func (s *MemoryStore) SetRole(_ context.Context, tenantID, memberID, role string) (*Member, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.guardLastOwner(tenantID, memberID, role)
	if err != nil {
		return nil, err
	}
	m.Role = role
	member := m.Member
	return &member, nil
}

func (s *MemoryStore) Remove(_ context.Context, tenantID, memberID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.guardLastOwner(tenantID, memberID, ""); err != nil {
		return err
	}
	delete(s.orgs[tenantID].members, memberID)
	return nil
}

// guardLastOwner returns the member, refusing changes that would leave the
// organization without an owner. newRole is "" for a removal.
func (s *MemoryStore) guardLastOwner(tenantID, memberID, newRole string) (*memMember, error) {
	org, ok := s.orgs[tenantID]
	if !ok {
		return nil, ErrNotFound
	}
	m, ok := org.members[memberID]
	if !ok {
		return nil, ErrNotFound
	}
	if m.Role == auth.RoleOwner && newRole != auth.RoleOwner {
		owners := 0
		for _, other := range org.members {
			if other.Role == auth.RoleOwner {
				owners++
			}
		}
		if owners <= 1 {
			return nil, ErrLastOwner
		}
	}
	return m, nil
}
