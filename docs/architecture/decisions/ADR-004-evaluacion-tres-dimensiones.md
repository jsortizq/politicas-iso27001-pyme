# ADR-004: Evaluación de controles en tres dimensiones separadas

**Estado:** Aceptada  
**Fecha:** 2026-11-11  
**Autor:** Juan Sebastián Ortiz Quintero

## Contexto

El SoA actual (`SOA_Matrix.json`) mezcla aplicabilidad e implementación
en un solo campo `implementation_status`. El resumen calcula porcentajes
que no coinciden con los registros individuales. Esto produce métricas
engañosas: un control marcado como "implementado" se cuenta como conforme,
aunque nadie haya verificado su efectividad.

## Decisión

Separar la evaluación de cada control en **tres dimensiones independientes**:

| Dimensión      | Valores                                |
|----------------|----------------------------------------|
| Aplicabilidad  | Pendiente, Aplicable, No aplicable     |
| Implementación | No iniciado, En progreso, Implementado |
| Efectividad    | No evaluado, Efectivo, No efectivo     |

## Justificación

- **Precisión normativa:** ISO 27001 requiere que la organización determine
  la aplicabilidad de cada control (cláusula 6.1.3 d), implemente los
  necesarios, y evalúe su desempeño (cláusula 9.1).
- **Transparencia:** el dashboard mostrará por separado avance de evaluación,
  avance de implementación, efectividad comprobada y controles pendientes
  de evidencia. No habrá un porcentaje único engañoso.
- **Rigor:** un control no aplicable requiere justificación documentada
  y aprobación. Un control implementado no aparece como efectivo hasta
  que se compruebe.
- **Auditoría:** un auditor puede verificar cada dimensión con evidencias
  independientes.

## Modelo de datos

```sql
CREATE TABLE control_assessments (
    id              UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id),
    control_id      UUID NOT NULL REFERENCES controls(id),
    applicability   TEXT NOT NULL DEFAULT 'pending'
                    CHECK (applicability IN ('pending','applicable','not_applicable')),
    implementation  TEXT NOT NULL DEFAULT 'not_started'
                    CHECK (implementation IN ('not_started','in_progress','implemented')),
    effectiveness   TEXT NOT NULL DEFAULT 'not_evaluated'
                    CHECK (effectiveness IN ('not_evaluated','effective','not_effective')),
    justification   TEXT,
    assessor_id     UUID REFERENCES users(id),
    assessed_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

## Consecuencias

- El campo `implementation_status` del SoA actual se migra a las tres
  dimensiones: `applicable` + el valor correspondiente de implementación.
- Los dashboards nunca muestran un porcentaje único de "cumplimiento".
- Cada cambio de evaluación genera un evento de auditoría.

## Alternativas descartadas

**Campo único con más valores** (ej: "implemented_verified",
"implemented_unverified"): mezcla dimensiones ortogonales y dificulta
filtrar o agrupar por cada una independientemente.
