import { useState } from 'react';
import { llamarApi, useConsulta } from '@shared/api';
import type { Caseta, ErrorApi, Persona, TipoPersona, Vehiculo } from '@shared/contrato';
import { TIPOS_PERSONA } from '@shared/contrato';
import { ETIQUETA_TIPO_PERSONA, normalizarPlaca, numero } from '@shared/formato';
import { IconoBuscar, IconoMas } from '@shared/iconos';
import { AvisoError, Boton, Campo, Cargando, claseEntrada, Pildora, Placa, Vacio } from '@shared/ui';
import { Casilla, EncabezadoPagina, FormularioLateral, Paginador, Panel, Tabla, useRetrasado } from '../comun';
import { useSesion } from '../sesion';

const POR_PAGINA = 10;

type Pestana = 'vehiculos' | 'personas' | 'casetas';

export function Catalogos() {
  const { puede } = useSesion();
  const pestanas: Array<[Pestana, string]> = [
    ...(puede('vehiculos.listar') ? [['vehiculos', 'Vehículos'] as [Pestana, string]] : []),
    ...(puede('personas.listar') ? [['personas', 'Personas'] as [Pestana, string]] : []),
    ...(puede('casetas.listar') ? [['casetas', 'Casetas'] as [Pestana, string]] : []),
  ];
  const [actual, setActual] = useState<Pestana>(pestanas[0]?.[0] ?? 'vehiculos');

  return (
    <>
      <EncabezadoPagina titulo="Catálogos" descripcion="Quién puede entrar, con qué vehículo y por qué caseta." />
      <div role="group" aria-label="Catálogo" className="flex flex-wrap gap-1 border-b-2 border-regla">
        {pestanas.map(([id, nombre]) => (
          <button
            key={id}
            type="button"
            aria-pressed={actual === id}
            onClick={() => setActual(id)}
            className={`-mb-0.5 min-h-12 px-4 text-[17px] ${actual === id ? 'font-semibold text-tinta shadow-[inset_0_-4px_0_var(--color-amarillo)]' : 'font-medium text-gris'}`}
          >
            {nombre}
          </button>
        ))}
      </div>
      {actual === 'vehiculos' && <CatalogoVehiculos />}
      {actual === 'personas' && <CatalogoPersonas />}
      {actual === 'casetas' && <CatalogoCasetas />}
    </>
  );
}

function Buscador({ id, valor, onCambiar, texto }: { id: string; valor: string; onCambiar: (v: string) => void; texto: string }) {
  return (
    <div className="relative flex flex-[1_1_320px] items-center">
      <IconoBuscar tam={18} className="pointer-events-none absolute left-3 text-gris" />
      <input
        id={id}
        aria-label={texto}
        placeholder={texto}
        value={valor}
        onChange={(e) => onCambiar(e.target.value)}
        className={claseEntrada(undefined, 'w-full pl-10')}
      />
    </div>
  );
}

function SelectorTipo({ valor, onCambiar, todos = 'Todos los tipos', id }: { valor: string; onCambiar: (v: string) => void; todos?: string; id?: string }) {
  return (
    <select id={id} aria-label={id ? undefined : 'Tipo de usuario'} value={valor} onChange={(e) => onCambiar(e.target.value)} className={claseEntrada()}>
      <option value="">{todos}</option>
      {TIPOS_PERSONA.map((t) => (
        <option key={t} value={t}>
          {ETIQUETA_TIPO_PERSONA[t]}
        </option>
      ))}
    </select>
  );
}

/** Guarda un formulario y deja los errores por campo listos para pintarse. */
function useGuardar() {
  const [guardando, setGuardando] = useState(false);
  const [error, setError] = useState<ErrorApi | null>(null);
  async function guardar<T>(llamada: () => Promise<{ ok: true; datos: T } | { ok: false; error: ErrorApi }>, alTerminar: () => void) {
    setGuardando(true);
    setError(null);
    const r = await llamada();
    setGuardando(false);
    if (r.ok) alTerminar();
    else setError(r.error);
  }
  return { guardando, error, campos: error?.campos ?? {}, guardar, limpiar: () => setError(null) };
}

// ---------- Vehículos ----------

function CatalogoVehiculos() {
  const { puede } = useSesion();
  const [texto, setTexto] = useState('');
  const [tipo, setTipo] = useState('');
  const [estado, setEstado] = useState('');
  const [pagina, setPagina] = useState(1);
  const [editando, setEditando] = useState<Vehiculo | 'nuevo' | null>(null);
  const busqueda = useRetrasado(texto);

  const lista = useConsulta('vehiculos.listar', {
    ...(busqueda.trim() ? { texto: busqueda.trim() } : {}),
    ...(tipo ? { tipoPersona: tipo as TipoPersona } : {}),
    ...(estado ? { autorizado: estado === 'si' } : {}),
    pagina,
    porPagina: POR_PAGINA,
  });

  const conPagina1 = (fn: (v: string) => void) => (v: string) => {
    fn(v);
    setPagina(1);
  };

  return (
    <>
      <div className="flex flex-wrap items-center gap-2.5">
        <Buscador id="buscar-vehiculo" valor={texto} onCambiar={conPagina1(setTexto)} texto="Busca por placa, persona, marca o modelo" />
        <SelectorTipo valor={tipo} onCambiar={conPagina1(setTipo)} />
        <select aria-label="Estado" value={estado} onChange={(e) => conPagina1(setEstado)(e.target.value)} className={claseEntrada()}>
          <option value="">Autorizados y no autorizados</option>
          <option value="si">Solo autorizados</option>
          <option value="no">Solo no autorizados</option>
        </select>
        {puede('vehiculos.crear') && (
          <Boton onClick={() => setEditando('nuevo')}>
            <IconoMas tam={18} />
            Agregar vehículo
          </Boton>
        )}
      </div>

      <div className="flex flex-wrap items-start gap-6">
        <Panel className="flex-[2_1_560px]">
          {lista.error && <AvisoError error={lista.error} onReintentar={lista.recargar} />}
          {!lista.datos && !lista.error && <Cargando />}
          {lista.datos?.total === 0 && <Vacio>No hay vehículos con esa búsqueda.</Vacio>}
          {lista.datos && lista.datos.total > 0 && (
            <>
              <Tabla<Vehiculo>
                minAncho={680}
                filas={lista.datos.elementos}
                clave={(v) => v.placa}
                columnas={[
                  { titulo: 'Placa', celda: (v) => <Placa placa={v.placa} /> },
                  { titulo: 'Persona', celda: (v) => v.nombrePersona },
                  { titulo: 'Tipo', celda: (v) => ETIQUETA_TIPO_PERSONA[v.tipoPersona] },
                  { titulo: 'Vehículo', celda: (v) => <span className="text-gris">{[v.marca, v.modelo, v.color].filter(Boolean).join(' ')}</span> },
                  {
                    titulo: 'Estado',
                    celda: (v) => (v.autorizado ? <Pildora tono="ok">Autorizado</Pildora> : <Pildora tono="error">No autorizado</Pildora>),
                  },
                  ...(puede('vehiculos.actualizar')
                    ? [{ titulo: 'Acciones', celda: (v: Vehiculo) => <BotonEditar onClick={() => setEditando(v)} /> }]
                    : []),
                ]}
              />
              <Paginador pagina={pagina} porPagina={POR_PAGINA} total={lista.datos.total} onCambiar={setPagina} />
            </>
          )}
        </Panel>

        {editando && (
          <FormVehiculo
            key={editando === 'nuevo' ? 'nuevo' : editando.placa}
            vehiculo={editando === 'nuevo' ? null : editando}
            onCancelar={() => setEditando(null)}
            onGuardado={() => {
              setEditando(null);
              void lista.recargar();
            }}
          />
        )}
      </div>
    </>
  );
}

function BotonEditar({ onClick }: { onClick: () => void }) {
  return (
    <Boton variante="texto" onClick={onClick}>
      Editar
    </Boton>
  );
}

function FormVehiculo({ vehiculo, onGuardado, onCancelar }: { vehiculo: Vehiculo | null; onGuardado: () => void; onCancelar: () => void }) {
  const [placa, setPlaca] = useState(vehiculo?.placa ?? '');
  const [persona, setPersona] = useState<{ id: string; texto: string } | null>(
    vehiculo ? { id: vehiculo.personaId, texto: `${vehiculo.nombrePersona}, ${ETIQUETA_TIPO_PERSONA[vehiculo.tipoPersona].toLowerCase()}` } : null,
  );
  const [marca, setMarca] = useState(vehiculo?.marca ?? '');
  const [modelo, setModelo] = useState(vehiculo?.modelo ?? '');
  const [color, setColor] = useState(vehiculo?.color ?? '');
  const [autorizado, setAutorizado] = useState(vehiculo?.autorizado ?? true);
  const { guardando, error, campos, guardar } = useGuardar();

  function onGuardar() {
    const comunes = { marca, modelo, color, autorizado };
    void guardar(
      () =>
        vehiculo
          ? llamarApi('vehiculos.actualizar', { placa: vehiculo.placa, personaId: persona?.id ?? vehiculo.personaId, ...comunes })
          : llamarApi('vehiculos.crear', { placa, personaId: persona?.id ?? '', ...comunes }),
      onGuardado,
    );
  }

  return (
    <FormularioLateral
      titulo={vehiculo ? `Editar ${vehiculo.placa}` : 'Nuevo vehículo'}
      textoGuardar="Guardar vehículo"
      guardando={guardando}
      error={error}
      onGuardar={onGuardar}
      onCancelar={onCancelar}
    >
      <Campo
        id="v-placa"
        etiqueta="Placa"
        error={campos.placa}
        ayuda={vehiculo ? 'La placa no se puede cambiar.' : placa && `Se guarda como ${normalizarPlaca(placa)}`}
      >
        <input
          id="v-placa"
          value={placa}
          disabled={Boolean(vehiculo)}
          maxLength={12}
          onChange={(e) => setPlaca(e.target.value)}
          aria-invalid={Boolean(campos.placa)}
          className={claseEntrada(campos.placa, 'min-h-13 font-display text-[26px] font-bold tracking-[3px] uppercase disabled:bg-concreto')}
        />
      </Campo>
      <BuscadorPersona seleccion={persona} onElegir={setPersona} error={campos.personaId} />
      <div className="flex flex-wrap gap-3">
        <Campo id="v-marca" etiqueta="Marca" className="flex-[1_1_110px]">
          <input id="v-marca" value={marca} maxLength={40} onChange={(e) => setMarca(e.target.value)} className={claseEntrada()} />
        </Campo>
        <Campo id="v-modelo" etiqueta="Modelo" className="flex-[1_1_110px]">
          <input id="v-modelo" value={modelo} maxLength={40} onChange={(e) => setModelo(e.target.value)} className={claseEntrada()} />
        </Campo>
        <Campo id="v-color" etiqueta="Color" className="flex-[1_1_110px]">
          <input id="v-color" value={color} maxLength={40} onChange={(e) => setColor(e.target.value)} className={claseEntrada()} />
        </Campo>
      </div>
      <Casilla
        etiqueta="Autorizado para entrar"
        ayuda="Sin autorización, la caseta verá un aviso al registrarlo."
        marcada={autorizado}
        onCambiar={setAutorizado}
      />
    </FormularioLateral>
  );
}

function BuscadorPersona({
  seleccion,
  onElegir,
  error,
}: {
  seleccion: { id: string; texto: string } | null;
  onElegir: (p: { id: string; texto: string } | null) => void;
  error?: string;
}) {
  const [texto, setTexto] = useState('');
  const busqueda = useRetrasado(texto);
  const resultados = useConsulta('personas.listar', { texto: busqueda.trim(), porPagina: 6 }, !seleccion && busqueda.trim().length >= 2);

  if (seleccion) {
    return (
      <Campo id="v-persona" etiqueta="Persona" error={error}>
        <div className="flex min-h-11 items-center justify-between gap-3 rounded-md bg-concreto px-3">
          <span id="v-persona" className="font-semibold">
            {seleccion.texto}
          </span>
          <Boton variante="texto" onClick={() => onElegir(null)}>
            Cambiar
          </Boton>
        </div>
      </Campo>
    );
  }

  const opciones = busqueda.trim().length >= 2 ? (resultados.datos?.elementos ?? []) : [];
  return (
    <Campo id="v-persona" etiqueta="Persona" error={error} ayuda="Busca por nombre o matrícula; escribe al menos 2 letras.">
      <input
        id="v-persona"
        value={texto}
        onChange={(e) => setTexto(e.target.value)}
        autoComplete="off"
        aria-invalid={Boolean(error)}
        className={claseEntrada(error)}
      />
      {opciones.length > 0 && (
        <ul className="flex flex-col rounded-md border-[1.5px] border-regla">
          {opciones.map((p: Persona) => (
            <li key={p.id} className="border-b border-concreto last:border-b-0">
              <button
                type="button"
                onClick={() => onElegir({ id: p.id, texto: `${p.nombre}, ${ETIQUETA_TIPO_PERSONA[p.tipo].toLowerCase()}${p.identificador ? `, ${p.identificador}` : ''}` })}
                className="flex min-h-11 w-full items-center justify-between gap-3 px-3 text-left hover:bg-concreto"
              >
                <span className="font-semibold">{p.nombre}</span>
                <span className="text-[15px] text-gris">
                  {ETIQUETA_TIPO_PERSONA[p.tipo]}
                  {p.identificador && `, ${p.identificador}`}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
      {busqueda.trim().length >= 2 && resultados.datos?.total === 0 && (
        <span className="text-[15px] text-gris">No hay personas con ese nombre. Agrégala en la pestaña Personas.</span>
      )}
    </Campo>
  );
}

// ---------- Personas ----------

function CatalogoPersonas() {
  const { puede } = useSesion();
  const [texto, setTexto] = useState('');
  const [tipo, setTipo] = useState('');
  const [pagina, setPagina] = useState(1);
  const [editando, setEditando] = useState<Persona | 'nuevo' | null>(null);
  const busqueda = useRetrasado(texto);
  const lista = useConsulta('personas.listar', {
    ...(busqueda.trim() ? { texto: busqueda.trim() } : {}),
    ...(tipo ? { tipo: tipo as TipoPersona } : {}),
    incluirInactivos: true,
    pagina,
    porPagina: POR_PAGINA,
  });

  return (
    <>
      <div className="flex flex-wrap items-center gap-2.5">
        <Buscador
          id="buscar-persona"
          valor={texto}
          onCambiar={(v) => {
            setTexto(v);
            setPagina(1);
          }}
          texto="Busca por nombre, matrícula o número de empleado"
        />
        <SelectorTipo
          valor={tipo}
          onCambiar={(v) => {
            setTipo(v);
            setPagina(1);
          }}
        />
        {puede('personas.crear') && (
          <Boton onClick={() => setEditando('nuevo')}>
            <IconoMas tam={18} />
            Agregar persona
          </Boton>
        )}
      </div>
      <div className="flex flex-wrap items-start gap-6">
        <Panel className="flex-[2_1_560px]">
          {lista.error && <AvisoError error={lista.error} onReintentar={lista.recargar} />}
          {!lista.datos && !lista.error && <Cargando />}
          {lista.datos?.total === 0 && <Vacio>No hay personas con esa búsqueda.</Vacio>}
          {lista.datos && lista.datos.total > 0 && (
            <>
              <Tabla<Persona>
                filas={lista.datos.elementos}
                clave={(p) => p.id}
                columnas={[
                  { titulo: 'Nombre', celda: (p) => <span className="font-semibold">{p.nombre}</span> },
                  { titulo: 'Tipo', celda: (p) => ETIQUETA_TIPO_PERSONA[p.tipo] },
                  { titulo: 'Matrícula o número', celda: (p) => p.identificador ?? <span className="text-gris">No aplica</span> },
                  { titulo: 'Teléfono', celda: (p) => p.telefono ?? <span className="text-gris">Sin teléfono</span> },
                  { titulo: 'Estado', celda: (p) => (p.activo ? <Pildora tono="ok">Activa</Pildora> : <Pildora tono="neutro">Inactiva</Pildora>) },
                  ...(puede('personas.actualizar') ? [{ titulo: 'Acciones', celda: (p: Persona) => <BotonEditar onClick={() => setEditando(p)} /> }] : []),
                ]}
              />
              <Paginador pagina={pagina} porPagina={POR_PAGINA} total={lista.datos.total} onCambiar={setPagina} />
            </>
          )}
        </Panel>
        {editando && (
          <FormPersona
            key={editando === 'nuevo' ? 'nuevo' : editando.id}
            persona={editando === 'nuevo' ? null : editando}
            onCancelar={() => setEditando(null)}
            onGuardado={() => {
              setEditando(null);
              void lista.recargar();
            }}
          />
        )}
      </div>
    </>
  );
}

function FormPersona({ persona, onGuardado, onCancelar }: { persona: Persona | null; onGuardado: () => void; onCancelar: () => void }) {
  const [nombre, setNombre] = useState(persona?.nombre ?? '');
  const [tipo, setTipo] = useState<string>(persona?.tipo ?? '');
  const [identificador, setIdentificador] = useState(persona?.identificador ?? '');
  const [telefono, setTelefono] = useState(persona?.telefono ?? '');
  const [activo, setActivo] = useState(persona?.activo ?? true);
  const { guardando, error, campos, guardar } = useGuardar();

  function onGuardar() {
    const datos = { nombre, tipo: tipo as TipoPersona, identificador, telefono };
    void guardar(
      () => (persona ? llamarApi('personas.actualizar', { id: persona.id, ...datos, activo }) : llamarApi('personas.crear', datos)),
      onGuardado,
    );
  }

  return (
    <FormularioLateral
      titulo={persona ? 'Editar persona' : 'Nueva persona'}
      textoGuardar="Guardar persona"
      guardando={guardando}
      error={error}
      onGuardar={onGuardar}
      onCancelar={onCancelar}
    >
      <Campo id="p-nombre" etiqueta="Nombre completo" error={campos.nombre}>
        <input id="p-nombre" value={nombre} maxLength={100} onChange={(e) => setNombre(e.target.value)} className={claseEntrada(campos.nombre)} />
      </Campo>
      <Campo id="p-tipo" etiqueta="Tipo de usuario" error={campos.tipo}>
        <SelectorTipo id="p-tipo" valor={tipo} onCambiar={setTipo} todos="Selecciona el tipo" />
      </Campo>
      <Campo
        id="p-identificador"
        etiqueta="Matrícula o número de empleado"
        error={campos.identificador}
        ayuda={tipo === 'VISITANTE' ? 'Opcional para visitantes.' : undefined}
      >
        <input id="p-identificador" value={identificador} onChange={(e) => setIdentificador(e.target.value)} className={claseEntrada(campos.identificador)} />
      </Campo>
      <Campo id="p-telefono" etiqueta="Teléfono" ayuda="Opcional.">
        <input id="p-telefono" type="tel" value={telefono} onChange={(e) => setTelefono(e.target.value)} className={claseEntrada()} />
      </Campo>
      {persona && <Casilla etiqueta="Persona activa" ayuda="Si la desactivas, ya no se le pueden asignar vehículos." marcada={activo} onCambiar={setActivo} />}
    </FormularioLateral>
  );
}

// ---------- Casetas ----------

function CatalogoCasetas() {
  const { puede } = useSesion();
  const [editando, setEditando] = useState<Caseta | 'nuevo' | null>(null);
  const lista = useConsulta('casetas.listar', { incluirInactivas: true });

  return (
    <>
      {puede('casetas.crear') && (
        <div>
          <Boton onClick={() => setEditando('nuevo')}>
            <IconoMas tam={18} />
            Agregar caseta
          </Boton>
        </div>
      )}
      <div className="flex flex-wrap items-start gap-6">
        <Panel className="flex-[2_1_560px]" descripcion={lista.datos && `${numero(lista.datos.length)} casetas registradas.`} titulo="Casetas">
          {lista.error && <AvisoError error={lista.error} onReintentar={lista.recargar} />}
          {!lista.datos && !lista.error && <Cargando />}
          {lista.datos && (
            <Tabla<Caseta>
              minAncho={520}
              filas={lista.datos}
              clave={(c) => c.id}
              columnas={[
                { titulo: 'Nombre', celda: (c) => <span className="font-semibold">{c.nombre}</span> },
                { titulo: 'Ubicación', celda: (c) => c.ubicacion ?? <span className="text-gris">Sin ubicación</span> },
                { titulo: 'Estado', celda: (c) => (c.activa ? <Pildora tono="ok">Activa</Pildora> : <Pildora tono="neutro">Inactiva</Pildora>) },
                ...(puede('casetas.actualizar') ? [{ titulo: 'Acciones', celda: (c: Caseta) => <BotonEditar onClick={() => setEditando(c)} /> }] : []),
              ]}
            />
          )}
        </Panel>
        {editando && (
          <FormCaseta
            key={editando === 'nuevo' ? 'nuevo' : editando.id}
            caseta={editando === 'nuevo' ? null : editando}
            onCancelar={() => setEditando(null)}
            onGuardado={() => {
              setEditando(null);
              void lista.recargar();
            }}
          />
        )}
      </div>
    </>
  );
}

function FormCaseta({ caseta, onGuardado, onCancelar }: { caseta: Caseta | null; onGuardado: () => void; onCancelar: () => void }) {
  const [nombre, setNombre] = useState(caseta?.nombre ?? '');
  const [ubicacion, setUbicacion] = useState(caseta?.ubicacion ?? '');
  const [activa, setActiva] = useState(caseta?.activa ?? true);
  const { guardando, error, campos, guardar } = useGuardar();

  return (
    <FormularioLateral
      titulo={caseta ? `Editar ${caseta.nombre}` : 'Nueva caseta'}
      textoGuardar="Guardar caseta"
      guardando={guardando}
      error={error}
      onGuardar={() =>
        void guardar(
          () => (caseta ? llamarApi('casetas.actualizar', { id: caseta.id, nombre, ubicacion, activa }) : llamarApi('casetas.crear', { nombre, ubicacion })),
          onGuardado,
        )
      }
      onCancelar={onCancelar}
    >
      <Campo id="c-nombre" etiqueta="Nombre" error={campos.nombre}>
        <input id="c-nombre" value={nombre} maxLength={100} onChange={(e) => setNombre(e.target.value)} className={claseEntrada(campos.nombre)} />
      </Campo>
      <Campo id="c-ubicacion" etiqueta="Ubicación" ayuda="Por ejemplo, acceso principal.">
        <input id="c-ubicacion" value={ubicacion} maxLength={100} onChange={(e) => setUbicacion(e.target.value)} className={claseEntrada()} />
      </Campo>
      {caseta && <Casilla etiqueta="Caseta activa" ayuda="Las casetas inactivas no aparecen al comenzar turno." marcada={activa} onCambiar={setActiva} />}
    </FormularioLateral>
  );
}
