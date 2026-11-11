# ADR-001: Monolito modular en lugar de microservicios

**Estado:** Aceptada  
**Fecha:** 2026-11-11  
**Autor:** Juan Sebastián Ortiz Quintero

## Contexto

El repositorio actual es una CLI en Go con un único archivo `main.go` de ~630 líneas.
El objetivo es evolucionar hacia una plataforma SaaS multiempresa con API, frontend,
workers y el copiloto de IA, manteniendo la CLI funcional.

Se evaluaron dos alternativas:
1. Microservicios desde el inicio (un servicio por dominio).
2. Monolito modular con separación interna por dominios.

## Decisión

**Monolito modular.** Un solo binario Go para la API, otro para workers, y la CLI
existente como tercer binario. Los tres comparten paquetes de dominio en `internal/`.

## Justificación

- **Complejidad operacional:** los microservicios requieren orquestación, service
  mesh, trazabilidad distribuida y gestión de estado eventual. Para un equipo
  pequeño y un producto en fase inicial, el costo supera el beneficio.
- **Velocidad de desarrollo:** un monolito permite refactorizar, mover código y
  cambiar fronteras entre módulos sin coordinar despliegues independientes.
- **Reutilización:** la API y los workers necesitan la misma lógica de negocio
  (evaluación de controles, cálculo de riesgos, validación de evidencias).
  Compartir paquetes Go es natural en un monorepo.
- **Evolución:** los límites entre módulos (`internal/modules/`) permiten extraer
  servicios independientes cuando el volumen lo justifique, sin reescribir lógica.

## Consecuencias

- Todo el backend se despliega como una unidad (API + workers).
- Se necesita disciplina para mantener las fronteras entre módulos
  (sin imports circulares, interfaces claras).
- La base de datos es compartida; cada módulo accede solo a sus tablas.
- Cuando un módulo necesite escalar independientemente (ej: IA con GPU),
  se extraerá como servicio con su propia API interna.

## Alternativas descartadas

**Microservicios desde el inicio:** se descartó por la complejidad operacional
prematura, especialmente para un equipo de uno o dos desarrolladores.
Se reconsiderará cuando haya más de 5 desarrolladores activos o cuando
el módulo de IA requiera infraestructura de cómputo separada.
