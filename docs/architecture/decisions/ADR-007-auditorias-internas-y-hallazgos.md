# ADR-007: Auditorías internas, hallazgos y acciones correctivas

**Estado:** Aceptada  
**Fecha:** 2026-11-14  
**Autor:** Juan Sebastián Ortiz Quintero

## Contexto

ISO/IEC 27001 exige auditar el SGSI a intervalos planificados (cláusula 9.2):
definir alcance y criterios, conservar los resultados e informarlos a la
dirección. Ante una no conformidad (cláusula 10.2) hay que determinar su
causa, aplicar una acción, comprobar que funcionó y conservar todo ello como
información documentada. Un auditor de certificación pide ver ese rastro
completo, de la auditoría al cierre de cada hallazgo.

## Decisión

1. **La auditoría tiene un ciclo corto y explícito:**
   planificada → en curso → completada, o cancelada desde las dos primeras.
   Para completarla hay que escribir las conclusiones; para cancelarla, el
   motivo.
2. **Los hallazgos solo se registran con la auditoría en curso.** Al
   completarla, lo que se encontró queda fijo: descripción, evidencia,
   severidad y requisito ya no cambian. Lo que sigue vivo es el tratamiento.
3. **Cuatro severidades:** no conformidad mayor, no conformidad menor,
   observación y oportunidad de mejora. Solo las dos primeras exigen causa
   raíz y acción correctiva para poder cerrarse.
4. **El tratamiento es un ciclo propio del hallazgo:**
   abierto → en curso → implementado → cerrado, con reapertura. Quien edita
   puede llevarlo hasta *implementado*; **cerrar es verificar la eficacia**,
   exige una nota de verificación y queda reservado a propietarios y
   administradores. Reabrir exige explicar por qué.
5. **El hallazgo apunta a un control del Anexo A o a una cláusula** (texto
   libre, p. ej. "7.5.3"). El control se valida contra el catálogo, y el
   hallazgo aparece en la ficha de ese control y en el panel principal
   mientras no se cierre.
6. **Una auditoría que empezó no se borra.** Solo se elimina la que sigue
   planificada; las demás se cancelan y conservan sus hallazgos. La
   comprobación se hace dentro de la misma operación que borra, con la fila
   bloqueada, para que un inicio simultáneo no se pierda. La misma regla se
   aplicó al borrado de documentos frente a una aprobación simultánea.
7. **Numeración H-1, H-2… por auditoría**, asignada con la auditoría
   bloqueada para que dos altas simultáneas no repitan número.
8. **El vencimiento se calcula al leer** (fecha límite pasada y hallazgo sin
   cerrar); no se guarda como estado.
9. **El informe es una página de la aplicación preparada para imprimir.** El
   navegador la guarda como PDF; no se genera un archivo en el servidor.

## Alternativas consideradas

- **Permitir corregir un hallazgo después de completar la auditoría:** es
  cómodo, pero el informe dejaría de coincidir con lo que se comunicó. Un
  error se resuelve cerrando el hallazgo con una nota que lo explique.
- **Exigir que el auditor no pertenezca al área auditada:** la norma pide
  objetividad e imparcialidad, pero la plataforma no conoce el organigrama.
  Se registra el nombre del auditor líder y queda a la vista en el informe.
- **Exigir que quien cierra no sea quien implementó la acción:** bloquearía a
  las empresas con una sola persona administradora. Queda registrado quién
  cerró y cuándo.
- **Generar el PDF en el servidor:** garantiza un formato idéntico y permite
  archivarlo como evidencia de forma automática, pero añade un motor de
  renderizado. Se deja para el módulo de informes.

## Consecuencias

- La independencia del auditor y la separación entre quien implementa y
  quien verifica **no se imponen**: dependen de la organización.
- El informe refleja el estado de los hallazgos en el momento de abrirlo; no
  queda una copia congelada salvo que alguien guarde el PDF (por ejemplo,
  como evidencia).
- Los hallazgos vencidos se marcan en pantalla, pero aún no se avisa a nadie.
- No hay programa anual de auditoría ni lista de verificación por control:
  cada auditoría se planifica por separado con su lista de controles.
- La revisión por la dirección (cláusula 9.3), que recibe los resultados de
  las auditorías, todavía no está implementada.
