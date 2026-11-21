import type { ErrorApi } from './contrato';
import { IconoBarrera, IconoInfo } from './iconos';
import { Boton } from './ui';

export const URL_SALIR_GOOGLE = 'https://accounts.google.com/Logout';

const TITULOS: Partial<Record<ErrorApi['codigo'], string>> = {
  NO_REGISTRADO: 'Tu cuenta no tiene acceso',
  USUARIO_INACTIVO: 'Tu cuenta está desactivada',
  SIN_IDENTIDAD: 'No pudimos identificar tu cuenta',
  PERMISO_DENEGADO: 'Esta aplicación no es para tu rol',
  RED: 'No hay conexión con el servidor',
};

/** Pantalla completa para errores al abrir la app (NO_REGISTRADO, SIN_IDENTIDAD, RED…). */
export function PantallaSinAcceso({ error, onReintentar }: { error: ErrorApi; onReintentar: () => void }) {
  const esCuenta = error.codigo !== 'RED' && error.codigo !== 'ERROR_INTERNO';

  return (
    <div className="flex min-h-screen flex-col gap-5 bg-concreto px-4 py-6">
      <div className="flex items-baseline gap-2">
        <span className="font-display text-2xl font-bold">Acceso vehicular</span>
        <span className="text-[15px] text-gris">CUTM Sur</span>
      </div>

      <main className="mx-auto flex w-full max-w-md flex-1 flex-col justify-center gap-4">
        <section className="flex flex-col gap-3.5 rounded-md bg-white px-5 pt-7 pb-5">
          <span className="flex size-15 items-center justify-center rounded-full bg-error-suave text-error">
            <IconoBarrera tam={32} />
          </span>
          <h1 className="font-display text-4xl leading-tight font-bold">
            {TITULOS[error.codigo] ?? 'No se pudo abrir la aplicación'}
          </h1>
          <p className="text-[17px] leading-relaxed">{error.mensaje}</p>
          {error.codigo === 'NO_REGISTRADO' && (
            <p className="text-[17px] leading-relaxed">Pide a un administrador que te registre con tu rol y tu caseta.</p>
          )}
          <div className="mt-1.5 flex flex-col gap-2.5">
            <Boton className="min-h-13 text-lg" onClick={onReintentar}>
              Reintentar acceso
            </Boton>
            {esCuenta && (
              <a
                href={URL_SALIR_GOOGLE}
                target="_top"
                onClick={(e) => import.meta.env.DEV && e.preventDefault()}
                className="btn inline-flex min-h-13 items-center justify-center rounded-lg border-2 border-tinta bg-white text-lg font-semibold text-tinta no-underline"
              >
                Usar otra cuenta
              </a>
            )}
          </div>
          <span className="text-sm text-gris">Código {error.codigo}</span>
        </section>

        {error.codigo === 'SIN_IDENTIDAD' && (
          <p className="flex gap-2.5 px-1 text-[15px] leading-relaxed">
            <IconoInfo className="mt-0.5 shrink-0" />
            Abre la app con tu cuenta institucional y cierra las demás cuentas de Google del navegador.
          </p>
        )}
      </main>
    </div>
  );
}
