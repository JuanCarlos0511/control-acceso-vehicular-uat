import { useEffect, useRef } from 'react';
import type { Usuario } from '@shared/contrato';
import { hora, iniciales, plural } from '@shared/formato';
import { IconoCerrar, IconoSalida } from '@shared/iconos';
import { URL_SALIR_GOOGLE } from '@shared/PantallaSinAcceso';
import { Boton } from '@shared/ui';
import type { Turno } from './Trabajo';

interface Props {
  usuario: Usuario;
  turno: Turno;
  cuenta: { entradas: number; salidas: number };
  onCerrar: () => void;
  onTerminar: () => void;
  onCambiarCaseta: () => void;
}

/** Panel inferior con los datos del turno; reemplaza al encabezado. */
export function OpcionesTurno({ usuario, turno, cuenta, onCerrar, onTerminar, onCambiarCaseta }: Props) {
  const cerrar = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    cerrar.current?.focus();
    const alTeclear = (e: KeyboardEvent) => e.key === 'Escape' && onCerrar();
    document.addEventListener('keydown', alTeclear);
    return () => document.removeEventListener('keydown', alTeclear);
  }, [onCerrar]);

  const { caseta } = turno;

  return (
    <div className="fixed inset-0 z-10 flex items-end justify-center">
      <div aria-hidden="true" className="absolute inset-0 bg-tinta/60" onClick={onCerrar} />
      <section
        role="dialog"
        aria-modal="true"
        aria-labelledby="turno-titulo"
        className="relative flex w-full max-w-md flex-col gap-4.5 rounded-t-2xl bg-white px-5 pt-2.5 pb-7"
      >
        <span aria-hidden="true" className="h-1 w-10 self-center rounded-full bg-regla" />

        <div className="flex items-center gap-3">
          <span
            aria-hidden="true"
            className="flex size-13 items-center justify-center rounded-full bg-amarillo font-display text-[22px] font-bold"
          >
            {iniciales(usuario.nombre)}
          </span>
          <div className="flex min-w-0 flex-1 flex-col">
            <h2 id="turno-titulo" className="font-display text-[30px] leading-tight font-bold">
              {usuario.nombre}
            </h2>
            <span className="truncate text-[15px] text-gris">{usuario.correo}</span>
          </div>
          <button
            ref={cerrar}
            type="button"
            aria-label="Cerrar opciones del turno"
            onClick={onCerrar}
            className="flex size-11 items-center justify-center rounded-full bg-concreto text-tinta"
          >
            <IconoCerrar />
          </button>
        </div>

        <dl className="flex flex-col border-t-2 border-tinta">
          <Fila etiqueta="Caseta" valor={caseta.ubicacion ? `${caseta.nombre}, ${caseta.ubicacion.toLowerCase()}` : caseta.nombre} />
          <Fila etiqueta="Turno" valor={`Desde las ${hora(turno.desde)}`} />
          <Fila etiqueta="En este turno" valor={`${plural(cuenta.entradas, 'entrada')}, ${plural(cuenta.salidas, 'salida')}`} />
        </dl>

        <div className="flex flex-col gap-2">
          {!usuario.casetaId && (
            <Boton variante="secundario" className="min-h-12" onClick={onCambiarCaseta}>
              Cambiar de caseta
            </Boton>
          )}
          <a
            href={URL_SALIR_GOOGLE}
            target="_top"
            onClick={(e) => {
              if (import.meta.env.DEV) {
                e.preventDefault();
                onTerminar();
              }
            }}
            className="btn flex min-h-14 items-center justify-center gap-3 rounded-lg bg-asfalto font-display text-2xl font-bold text-white no-underline"
          >
            <IconoSalida tam={24} />
            Terminar turno
          </a>
          <p className="text-center text-sm text-gris">Se cerrará tu cuenta de Google en este teléfono.</p>
        </div>
      </section>
    </div>
  );
}

function Fila({ etiqueta, valor }: { etiqueta: string; valor: string }) {
  return (
    <div className="flex justify-between gap-3 border-b border-concreto py-3">
      <dt className="text-gris">{etiqueta}</dt>
      <dd className="text-right font-semibold">{valor}</dd>
    </div>
  );
}
