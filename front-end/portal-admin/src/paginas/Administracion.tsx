import { useState, type FormEvent } from 'react';
import { llamarApi, useConsulta } from '@shared/api';
import type { EntradaBitacora, ErrorApi, Rol, Usuario } from '@shared/contrato';
import { ROLES } from '@shared/contrato';
import { etiquetaAccion, ETIQUETA_ROL, fechaHoraCorta, hoy } from '@shared/formato';
import { IconoMas } from '@shared/iconos';
import { AvisoError, Boton, Campo, Cargando, claseEntrada, Pildora, Vacio } from '@shared/ui';
import { Casilla, EncabezadoPagina, FormularioLateral, Paginador, Panel, SelectorCaseta, Tabla } from '../comun';
import { useSesion } from '../sesion';

const POR_PAGINA = 15;

const ACTIVIDADES: Array<[string, string]> = [
  ['', 'Todas las actividades'],
  ['ACCESO', 'Abrió la aplicación'],
  ['movimientos.registrar', 'Registró un movimiento'],
  ['bitacora.registrarExportacion', 'Exportó datos'],
  ['vehiculos.crear', 'Agregó un vehículo'],
  ['vehiculos.actualizar', 'Editó un vehículo'],
  ['personas.crear', 'Agregó una persona'],
  ['usuarios.crear', 'Agregó un usuario'],
  ['usuarios.actualizar', 'Editó un usuario'],
];

export function Administracion() {
  const { puede } = useSesion();
  return (
    <>
      <EncabezadoPagina titulo="Administración" descripcion="Quién usa el sistema y qué ha hecho cada quien." />
      {puede('usuarios.listar') && <Usuarios />}
      {puede('bitacora.listar') && <Bitacora />}
    </>
  );
}

function Usuarios() {
  const { puede, casetas } = useSesion();
  const [editando, setEditando] = useState<Usuario | 'nuevo' | null>(null);
  const lista = useConsulta('usuarios.listar', { incluirInactivos: true });
  const nombreCaseta = (id: string | null) => (id ? (casetas.find((c) => c.id === id)?.nombre ?? id) : 'Sin caseta fija');

  return (
    <div className="flex flex-wrap items-start gap-6">
      <Panel
        titulo="Usuarios del sistema"
        descripcion="Entran con su cuenta de Google institucional. Aquí defines su rol y su caseta."
        acciones={
          puede('usuarios.crear') && (
            <Boton onClick={() => setEditando('nuevo')}>
              <IconoMas tam={18} />
              Agregar usuario
            </Boton>
          )
        }
        className="flex-[2_1_620px]"
      >
        {lista.error && <AvisoError error={lista.error} onReintentar={lista.recargar} />}
        {!lista.datos && !lista.error && <Cargando />}
        {lista.datos && (
          <Tabla<Usuario>
            minAncho={760}
            filas={lista.datos}
            clave={(u) => u.correo}
            columnas={[
              { titulo: 'Correo', celda: (u) => u.correo },
              { titulo: 'Nombre', celda: (u) => <span className="font-semibold">{u.nombre}</span> },
              { titulo: 'Rol', celda: (u) => ETIQUETA_ROL[u.rol] },
              { titulo: 'Caseta', celda: (u) => <span className="text-gris">{u.rol === 'GUARDIA' ? nombreCaseta(u.casetaId) : 'Todas'}</span> },
              { titulo: 'Estado', celda: (u) => (u.activo ? <Pildora tono="ok">Activo</Pildora> : <Pildora tono="neutro">Inactivo</Pildora>) },
              ...(puede('usuarios.actualizar')
                ? [
                    {
                      titulo: 'Acciones',
                      celda: (u: Usuario) => (
                        <Boton variante="texto" onClick={() => setEditando(u)}>
                          Editar
                        </Boton>
                      ),
                    },
                  ]
                : []),
            ]}
          />
        )}
      </Panel>
      {editando && (
        <FormUsuario
          key={editando === 'nuevo' ? 'nuevo' : editando.correo}
          usuario={editando === 'nuevo' ? null : editando}
          onCancelar={() => setEditando(null)}
          onGuardado={() => {
            setEditando(null);
            void lista.recargar();
          }}
        />
      )}
    </div>
  );
}

function FormUsuario({ usuario, onGuardado, onCancelar }: { usuario: Usuario | null; onGuardado: () => void; onCancelar: () => void }) {
  const [correo, setCorreo] = useState(usuario?.correo ?? '');
  const [nombre, setNombre] = useState(usuario?.nombre ?? '');
  const [rol, setRol] = useState<Rol>(usuario?.rol ?? 'GUARDIA');
  const [casetaId, setCasetaId] = useState(usuario?.casetaId ?? '');
  const [activo, setActivo] = useState(usuario?.activo ?? true);
  const [guardando, setGuardando] = useState(false);
  const [error, setError] = useState<ErrorApi | null>(null);
  const campos = error?.campos ?? {};

  async function onGuardar() {
    setGuardando(true);
    setError(null);
    const caseta = rol === 'GUARDIA' && casetaId ? casetaId : null;
    const r = usuario
      ? await llamarApi('usuarios.actualizar', { correo: usuario.correo, nombre, rol, casetaId: caseta, activo })
      : await llamarApi('usuarios.crear', { correo, nombre, rol, ...(caseta ? { casetaId: caseta } : {}) });
    setGuardando(false);
    if (r.ok) onGuardado();
    else setError(r.error);
  }

  return (
    <FormularioLateral
      titulo={usuario ? 'Editar usuario' : 'Nuevo usuario'}
      textoGuardar="Guardar usuario"
      guardando={guardando}
      error={error}
      onGuardar={() => void onGuardar()}
      onCancelar={onCancelar}
    >
      <Campo id="u-correo" etiqueta="Correo institucional" error={campos.correo} ayuda={usuario ? 'El correo no se puede cambiar.' : undefined}>
        <input
          id="u-correo"
          type="email"
          value={correo}
          disabled={Boolean(usuario)}
          onChange={(e) => setCorreo(e.target.value)}
          className={claseEntrada(campos.correo, 'disabled:bg-concreto')}
        />
      </Campo>
      <Campo id="u-nombre" etiqueta="Nombre" error={campos.nombre}>
        <input id="u-nombre" value={nombre} maxLength={100} onChange={(e) => setNombre(e.target.value)} className={claseEntrada(campos.nombre)} />
      </Campo>
      <Campo id="u-rol" etiqueta="Rol" error={campos.rol}>
        <select id="u-rol" value={rol} onChange={(e) => setRol(e.target.value as Rol)} className={claseEntrada(campos.rol)}>
          {ROLES.map((r) => (
            <option key={r} value={r}>
              {ETIQUETA_ROL[r]}
            </option>
          ))}
        </select>
      </Campo>
      {rol === 'GUARDIA' && (
        <Campo id="u-caseta" etiqueta="Caseta" error={campos.casetaId} ayuda="Sin caseta fija, el guardia la elige al comenzar turno.">
          <SelectorCaseta id="u-caseta" valor={casetaId} onCambiar={setCasetaId} todas="Sin caseta fija" />
        </Campo>
      )}
      {usuario && <Casilla etiqueta="Usuario activo" ayuda="Si lo desactivas, ya no podrá abrir el sistema." marcada={activo} onCambiar={setActivo} />}
    </FormularioLateral>
  );
}

function Bitacora() {
  const inicial = { desde: hoy(), hasta: hoy(), usuario: '', accion: '' };
  const [borrador, setBorrador] = useState(inicial);
  const [filtros, setFiltros] = useState(inicial);
  const [pagina, setPagina] = useState(1);
  const lista = useConsulta('bitacora.listar', {
    ...(filtros.desde ? { desde: filtros.desde } : {}),
    ...(filtros.hasta ? { hasta: filtros.hasta } : {}),
    ...(filtros.usuario.trim() ? { usuario: filtros.usuario.trim() } : {}),
    ...(filtros.accion ? { accion: filtros.accion } : {}),
    pagina,
    porPagina: POR_PAGINA,
  });

  function buscar(e: FormEvent) {
    e.preventDefault();
    setFiltros(borrador);
    setPagina(1);
  }

  return (
    <Panel titulo="Bitácora de actividades" descripcion="Cada registro, cambio, exportación e intento rechazado.">
      <form onSubmit={buscar} aria-label="Filtros de la bitácora" className="mb-4 flex flex-wrap items-end gap-2.5">
        <Campo id="b-desde" etiqueta="Desde">
          <input id="b-desde" type="date" value={borrador.desde} onChange={(e) => setBorrador({ ...borrador, desde: e.target.value })} className={claseEntrada()} />
        </Campo>
        <Campo id="b-hasta" etiqueta="Hasta">
          <input id="b-hasta" type="date" value={borrador.hasta} onChange={(e) => setBorrador({ ...borrador, hasta: e.target.value })} className={claseEntrada()} />
        </Campo>
        <Campo id="b-usuario" etiqueta="Usuario" className="flex-[1_1_220px]">
          <input id="b-usuario" value={borrador.usuario} placeholder="Correo o parte" onChange={(e) => setBorrador({ ...borrador, usuario: e.target.value })} className={claseEntrada()} />
        </Campo>
        <Campo id="b-accion" etiqueta="Actividad" className="flex-[1_1_220px]">
          <select id="b-accion" value={borrador.accion} onChange={(e) => setBorrador({ ...borrador, accion: e.target.value })} className={claseEntrada()}>
            {ACTIVIDADES.map(([valor, texto]) => (
              <option key={valor} value={valor}>
                {texto}
              </option>
            ))}
          </select>
        </Campo>
        <Boton type="submit">Buscar actividad</Boton>
      </form>

      {lista.error && <AvisoError error={lista.error} onReintentar={lista.recargar} />}
      {!lista.datos && !lista.error && <Cargando />}
      {lista.datos?.total === 0 && <Vacio>No hay actividad con esos filtros.</Vacio>}
      {lista.datos && lista.datos.total > 0 && (
        <>
          <Tabla<EntradaBitacora>
            minAncho={900}
            filas={lista.datos.elementos}
            clave={(b, i) => `${i}-${b.fechaHora}-${b.usuario}`}
            columnas={[
              { titulo: 'Fecha y hora', celda: (b) => <span className="text-gris">{fechaHoraCorta(b.fechaHora)}</span> },
              { titulo: 'Usuario', celda: (b) => b.usuario },
              { titulo: 'Actividad', celda: (b) => <span className="font-semibold">{etiquetaAccion(b.accion)}</span> },
              { titulo: 'Resultado', celda: (b) => (b.resultado === 'OK' ? <Pildora tono="ok">Hecho</Pildora> : <Pildora tono="error">Rechazado</Pildora>) },
              { titulo: 'Detalle', celda: (b) => b.detalle },
            ]}
          />
          <Paginador pagina={pagina} porPagina={POR_PAGINA} total={lista.datos.total} onCambiar={setPagina} />
        </>
      )}
    </Panel>
  );
}
