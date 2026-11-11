# ADR-003: AI Gateway interno en lugar de conexión directa a proveedores

**Estado:** Aceptada  
**Fecha:** 2026-11-11  
**Autor:** Juan Sebastián Ortiz Quintero

## Contexto

La plataforma incluirá un copiloto de IA que asiste en la gestión del SGSI:
generación de borradores de políticas, análisis de brechas, clasificación
de evidencias y consultas sobre el marco normativo.

Se evaluó si los módulos deberían conectarse directamente a la API de un
proveedor de modelos (OpenAI, Anthropic, etc.) o a través de una capa
intermedia propia.

## Decisión

**AI Gateway interno.** Un módulo `internal/modules/ai/` que centraliza
toda comunicación con proveedores de modelos. Ningún otro módulo accede
directamente a APIs de IA.

## Justificación

- **Independencia de proveedor:** se puede cambiar de modelo o proveedor
  sin modificar los módulos de negocio.
- **Control de costos:** el gateway aplica cuotas por empresa y por
  funcionalidad, previniendo costos inesperados.
- **Seguridad:** el gateway garantiza que cada consulta RAG filtre
  documentos por `organization_id` antes de enviarlos al modelo.
- **Trazabilidad:** registra modelo usado, versión del prompt, fuentes
  consultadas, usuario, consumo de tokens y timestamp.
- **Tratamiento de datos:** se evalúan las políticas de retención y
  uso para entrenamiento de cada proveedor.

## Implementación

El gateway expone una interfaz Go que los módulos consumen:

```go
type AIGateway interface {
    Assist(ctx context.Context, req AssistRequest) (*AssistResponse, error)
    GenerateDocument(ctx context.Context, req DocGenRequest) (*DocGenResponse, error)
    AnalyzeGap(ctx context.Context, req GapRequest) (*GapResponse, error)
}
```

Internamente usa adaptadores para cada proveedor. El adaptador
se selecciona por configuración.

## Consecuencias

- Los módulos de negocio no conocen qué modelo se usa.
- Cambiar de proveedor es un cambio de configuración, no de código.
- Se necesita una tabla `ai_usage` para rastrear consumo por empresa.
- El gateway añade una capa de latencia mínima (validación + logging).

## Reglas de seguridad

1. Documentos de una empresa nunca se usan para responder a otra.
2. Documentos subidos se tratan como datos, no como instrucciones.
3. La IA no puede aprobar controles, cerrar incidentes ni aceptar riesgos.
4. Resultados importantes requieren revisión humana.
