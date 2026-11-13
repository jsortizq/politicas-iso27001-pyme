# ADR-006: Documentos controlados y evidencias

**Estado:** Aceptada  
**Fecha:** 2026-11-13  
**Autor:** Juan Sebastián Ortiz Quintero

## Contexto

ISO/IEC 27001 (cláusula 7.5) exige controlar la información documentada:
identificarla, revisarla y aprobarla, controlar sus cambios y conservarla.
Un auditor pide además evidencias de que los controles funcionan y necesita
confiar en que no se han alterado después de recogerlas.

## Decisión

1. **Dos conceptos separados.** Un *documento* (política, procedimiento,
   registro…) tiene versiones y pasa por aprobación. Una *evidencia* es un
   archivo único que demuestra algo en una fecha; no se versiona.
2. **Los archivos son inmutables.** Cada subida crea un objeto nuevo en el
   almacenamiento; nunca se sobrescribe. Un cambio en un documento es una
   versión nueva. La base de datos rechaza, con un disparador, cualquier
   cambio en la huella, la clave, el tamaño o el nombre de un archivo ya
   registrado.
3. **Huella SHA-256 calculada al recibir el archivo** y guardada con el
   registro. La comprobación de integridad vuelve a leer el objeto y compara.
   Al eliminar una evidencia, su huella queda en el registro de actividad.
4. **Flujo de aprobación por versión:** borrador → en revisión → aprobada o
   rechazada (con motivo). Solo avanza la versión más reciente. Aprobar una
   versión sustituye a la vigente; un índice único impide que haya dos
   aprobadas. Propietarios y administradores aprueban.
5. **Un documento que llegó a aprobarse no se borra:** se marca obsoleto y
   queda congelado, con sus versiones disponibles para consulta.
6. **Vínculo con los controles del Anexo A**, no con las entradas del SoA de
   cada empresa, para que el respaldo documental no dependa de que el
   control esté evaluado.
7. **Las descargas pasan por la API**, que comprueba sesión y organización
   en cada una. No se generan enlaces públicos ni prefirmados.
8. **Los archivos nunca se interpretan en el navegador:** se sirven como
   adjunto, con tipo genérico, `nosniff` y una CSP `sandbox`. Por eso se
   acepta cualquier tipo de archivo sin lista de extensiones.
9. **Almacenamiento detrás de una interfaz** (`internal/storage`) con dos
   implementaciones: S3 compatible y disco local.

## Alternativas consideradas

- **Enlaces prefirmados de S3:** descargan directo del almacenamiento y
  alivian a la API, pero el enlace funciona para cualquiera que lo tenga
  hasta que caduca y la descarga no pasa por el control de acceso. Se puede
  añadir más adelante para archivos grandes.
- **Guardar los archivos en PostgreSQL:** simplifica las copias de seguridad
  pero hace crecer la base de datos y complica las descargas en streaming.
- **Exigir que quien aprueba no sea quien subió la versión:** es buena
  práctica, pero bloquea a las empresas con una sola persona a cargo. Queda
  registrado quién subió y quién aprobó, de modo que un auditor puede verlo.
- **SDK de AWS:** añade muchas dependencias para tres operaciones. La firma
  Signature V4 propia se comprueba contra los ejemplos publicados por AWS y,
  en CI, contra un MinIO real.

## Consecuencias

- **No hay análisis antivirus.** Un archivo malicioso subido por un miembro
  se entregará tal cual a quien lo descargue.
- La comprobación de integridad detecta alteraciones en el almacenamiento,
  pero quien controle a la vez el almacenamiento y la base de datos podría
  cambiar ambos. Para un sello más fuerte haría falta firma o sellado de
  tiempo externo.
- Un mismo usuario con rol de propietario o administrador puede subir y
  aprobar su propia versión.
- Si falla el borrado en el almacenamiento tras borrar el registro, queda un
  objeto huérfano (se registra en el log); no hay limpieza automática.
- El archivo completo pasa por la API y por un archivo temporal: adecuado
  para documentos de oficina, no para archivos de varios gigabytes.
- Las evidencias vencidas se marcan, pero aún no se avisa a nadie.
