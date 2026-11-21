import { useState } from 'react';
import type { Caseta, Inicio } from '@shared/contrato';
import { iniciales } from '@shared/formato';
import { IconoEntrada } from '@shared/iconos';
import { URL_SALIR_GOOGLE } from '@shared/PantallaSinAcceso';
import { Boton } from '@shared/ui';

export function ComenzarTurno({ inicio, onComenzar }: { inicio: Inicio; onComenzar: (caseta: Caseta) => void }) {
  const { usuario, casetas } = inicio;
  const fija = casetas.find((c) => c.id === usuario.casetaId) ?? null;
  const [elegida, setElegida] = useState(fija?.id ?? '');
  const caseta = fija ?? casetas.find((c) => c.id === elegida) ?? null;
  const nombre = usuario.nombre.split(' ')[0];

  return (
    <div className="mx-auto flex min-h-screen max-w-md flex-col gap-6 bg-white px-4 pt-6 pb-7">
      <div className="flex items-baseline gap-2">
        <span className="font-display text-[22px] font-bold">Acceso vehicular</span>
        <span className="text-[15px] text-gris">CUTM Sur</span>
      </div>

      <main className="flex flex-1 flex-col justify-center gap-6">
        <div className="flex flex-col gap-2.5">
          <span
            aria-hidden="true"
            className="flex size-16 items-center justify-center rounded-full bg-amarillo font-display text-[28px] font-bold"
          >
            {iniciales(usuario.nombre)}
          </span>
          <h1 className="font-display text-5xl leading-none font-bold">Hola, {nombre}</h1>
          <p className="text-[17px] text-gris">{usuario.correo}</p>
        </div>

        {fija ? (
          <div className="flex flex-col gap-0.5 rounded-lg bg-concreto px-5 py-4.5">
            <span className="text-[15px] text-gris">Tu caseta</span>
            <span className="font-display text-[34px] leading-tight font-bold">{fija.nombre}</span>
            {fija.ubicacion && <span>{fija.ubicacion}</span>}
          </div>
        ) : (
          <fieldset className="flex flex-col gap-2">
            <legend className="mb-2 text-[15px] font-semibold">¿En qué caseta vas a trabajar?</legend>
            {casetas.map((c) => {
              const activa = elegida === c.id;
              return (
                <label
                  key={c.id}
                  className={`flex min-h-14 cursor-pointer items-center gap-3 rounded-lg border-[1.5px] px-4 py-2 ${
                    activa ? 'border-asfalto bg-asfalto text-white' : 'border-regla'
                  }`}
                >
                  <input
                    type="radio"
                    name="caseta"
                    value={c.id}
                    checked={activa}
                    onChange={() => setElegida(c.id)}
                    className="size-5 accent-amarillo"
                  />
                  <span className="flex flex-col">
                    <span className="font-display text-2xl leading-tight font-bold">{c.nombre}</span>
                    {c.ubicacion && <span className="text-[15px]">{c.ubicacion}</span>}
                  </span>
                </label>
              );
            })}
          </fieldset>
        )}

        <div className="flex flex-col gap-2">
          <Boton
            className="min-h-16 font-display text-[26px]"
            disabled={!caseta}
            onClick={() => caseta && onComenzar(caseta)}
          >
            <IconoEntrada tam={26} />
            {caseta ? `Comenzar turno en ${caseta.nombre}` : 'Elige una caseta'}
          </Boton>
          <a
            href={URL_SALIR_GOOGLE}
            target="_top"
            onClick={(e) => import.meta.env.DEV && e.preventDefault()}
            className="inline-flex min-h-11 items-center self-center font-semibold text-tinta"
          >
            ¿No eres {nombre}? Usar otra cuenta
          </a>
        </div>
      </main>

      <p className="text-sm text-gris">Entraste con tu cuenta institucional de Google.</p>
    </div>
  );
}
