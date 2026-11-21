import { useCallback, useEffect, useState } from 'react';
import { llamarApi } from '@shared/api';
import type { ErrorApi, Inicio } from '@shared/contrato';
import { ETIQUETA_ROL } from '@shared/formato';
import { PantallaSinAcceso } from '@shared/PantallaSinAcceso';
import { Cargando } from '@shared/ui';
import { ComenzarTurno } from './ComenzarTurno';
import { Trabajo, type Turno } from './Trabajo';

type Estado =
  | { fase: 'cargando' }
  | { fase: 'error'; error: ErrorApi }
  | { fase: 'turno'; inicio: Inicio }
  | { fase: 'trabajo'; inicio: Inicio; turno: Turno };

export function App() {
  const [estado, setEstado] = useState<Estado>({ fase: 'cargando' });

  const iniciar = useCallback(async () => {
    setEstado({ fase: 'cargando' });
    const r = await llamarApi('app.iniciar', {});
    if (!r.ok) return setEstado({ fase: 'error', error: r.error });
    if (!r.datos.permisos.includes('movimientos.registrar')) {
      return setEstado({
        fase: 'error',
        error: {
          codigo: 'PERMISO_DENEGADO',
          mensaje: `Con el rol ${ETIQUETA_ROL[r.datos.usuario.rol].toLowerCase()} no se registran entradas ni salidas. Usa el portal de administración.`,
        },
      });
    }
    setEstado({ fase: 'turno', inicio: r.datos });
  }, []);

  useEffect(() => {
    void iniciar();
  }, [iniciar]);

  switch (estado.fase) {
    case 'cargando':
      return (
        <div className="mx-auto max-w-md px-4">
          <Cargando texto="Abriendo la caseta…" />
        </div>
      );
    case 'error':
      return <PantallaSinAcceso error={estado.error} onReintentar={iniciar} />;
    case 'turno':
      return (
        <ComenzarTurno
          inicio={estado.inicio}
          onComenzar={(caseta) =>
            setEstado({ fase: 'trabajo', inicio: estado.inicio, turno: { caseta, desde: new Date().toISOString() } })
          }
        />
      );
    case 'trabajo':
      return (
        <Trabajo
          inicio={estado.inicio}
          turno={estado.turno}
          onCambiarCaseta={() => setEstado({ fase: 'turno', inicio: estado.inicio })}
          onTerminar={() => setEstado({ fase: 'turno', inicio: estado.inicio })}
        />
      );
  }
}
