import { useCallback, useEffect, useState, type ComponentType } from 'react';
import { llamarApi } from '@shared/api';
import type { Accion, ErrorApi, Inicio } from '@shared/contrato';
import { ETIQUETA_ROL } from '@shared/formato';
import { IconoSalida } from '@shared/iconos';
import { PantallaSinAcceso, URL_SALIR_GOOGLE } from '@shared/PantallaSinAcceso';
import { Cargando } from '@shared/ui';
import { Administracion } from './paginas/Administracion';
import { Catalogos } from './paginas/Catalogos';
import { Consultas } from './paginas/Consultas';
import { Reportes } from './paginas/Reportes';
import { Tablero } from './paginas/Tablero';
import { SesionContext } from './sesion';

interface Pagina {
  id: string;
  nombre: string;
  /** Se muestra si el usuario tiene al menos uno de estos permisos. */
  permisos: Accion[];
  Componente: ComponentType;
}

const PAGINAS: Pagina[] = [
  { id: 'tablero', nombre: 'Tablero', permisos: ['reportes.resumen'], Componente: Tablero },
  { id: 'consultas', nombre: 'Consultas', permisos: ['movimientos.buscar'], Componente: Consultas },
  { id: 'reportes', nombre: 'Reportes', permisos: ['reportes.resumen'], Componente: Reportes },
  { id: 'catalogos', nombre: 'Catálogos', permisos: ['vehiculos.listar', 'personas.listar'], Componente: Catalogos },
  { id: 'administracion', nombre: 'Administración', permisos: ['usuarios.listar', 'bitacora.listar'], Componente: Administracion },
];

function useHash() {
  const [hash, setHash] = useState(() => location.hash.slice(1));
  useEffect(() => {
    const alCambiar = () => setHash(location.hash.slice(1));
    window.addEventListener('hashchange', alCambiar);
    return () => window.removeEventListener('hashchange', alCambiar);
  }, []);
  return hash;
}

export function App() {
  const [inicio, setInicio] = useState<Inicio | null>(null);
  const [error, setError] = useState<ErrorApi | null>(null);
  const hash = useHash();

  const iniciar = useCallback(async () => {
    setError(null);
    setInicio(null);
    const r = await llamarApi('app.iniciar', {});
    if (r.ok) setInicio(r.datos);
    else setError(r.error);
  }, []);

  useEffect(() => {
    void iniciar();
  }, [iniciar]);

  if (error) return <PantallaSinAcceso error={error} onReintentar={iniciar} />;
  if (!inicio) {
    return (
      <div className="mx-auto max-w-[1360px] px-8">
        <Cargando texto="Abriendo el panel…" />
      </div>
    );
  }

  const visibles = PAGINAS.filter((p) => p.permisos.some((a) => inicio.permisos.includes(a)));
  const actual = visibles.find((p) => p.id === hash) ?? visibles[0];

  return (
    <SesionContext.Provider value={inicio}>
      <div className="min-h-screen bg-concreto text-tinta">
        <header className="bg-asfalto text-white">
          <div className="mx-auto flex max-w-[1360px] flex-wrap items-center gap-x-7 px-4 sm:px-8">
            <a href={`#${visibles[0]?.id ?? ''}`} className="flex min-h-16 items-center gap-2 text-white no-underline">
              <span className="font-display text-2xl font-bold">Acceso vehicular</span>
              <span className="text-sm text-regla">CUTM Sur</span>
            </a>
            <nav aria-label="Principal" className="flex flex-auto flex-wrap gap-1">
              {visibles.map((p) => {
                const activa = p.id === actual?.id;
                return (
                  <a
                    key={p.id}
                    href={`#${p.id}`}
                    aria-current={activa ? 'page' : undefined}
                    className={`flex min-h-16 items-center px-3 no-underline ${
                      activa ? 'font-semibold text-white shadow-[inset_0_-4px_0_var(--color-amarillo)]' : 'font-medium text-regla hover:text-white'
                    }`}
                  >
                    {p.nombre}
                  </a>
                );
              })}
            </nav>
            <div className="flex min-h-16 items-center gap-4">
              <span className="text-[15px] text-regla">
                {inicio.usuario.nombre}, rol {ETIQUETA_ROL[inicio.usuario.rol].toLowerCase()}
              </span>
              <a
                href={URL_SALIR_GOOGLE}
                target="_top"
                onClick={(e) => import.meta.env.DEV && e.preventDefault()}
                className="flex min-h-11 items-center gap-1.5 text-[15px] font-semibold text-white no-underline"
              >
                <IconoSalida tam={18} />
                Cerrar sesión
              </a>
            </div>
          </div>
        </header>

        <main className="mx-auto flex max-w-[1360px] flex-col gap-6 px-4 pt-8 pb-14 sm:px-8">
          {actual ? (
            <actual.Componente key={actual.id} />
          ) : (
            <p className="text-gris">Tu rol no tiene secciones disponibles en el panel.</p>
          )}
        </main>
      </div>
    </SesionContext.Provider>
  );
}
