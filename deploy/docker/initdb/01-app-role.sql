-- Rol de aplicación para desarrollo local.
-- La API se conecta con sgsi_app, que NO es superusuario ni dueño de las
-- tablas, de modo que PostgreSQL aplica las políticas RLS (ADR-002).
-- En producción, crea este rol con una contraseña gestionada como secreto.

DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'app_user') THEN
        CREATE ROLE app_user NOLOGIN;
    END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'sgsi_app') THEN
        CREATE ROLE sgsi_app LOGIN PASSWORD 'sgsi_app_dev' IN ROLE app_user;
    END IF;
END
$$;
