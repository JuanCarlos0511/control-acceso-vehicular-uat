import { useRef, useState, type FormEvent, type ReactNode } from 'react';
import { llamarApi, useConsulta } from '@shared/api';
import type {
  Caseta,
  ErrorApi,
  Inicio,
  ResultadoRegistro,
  TipoMovimiento,
  TipoPersona,
  Usuario,
  VerificacionPlaca,
} from '@shared/contrato';
import { TIPOS_PERSONA } from '@shared/contrato';
import {
  cuando,
  duracion,
  ETIQUETA_TIPO_PERSONA,
  hora,
  horaConSegundos,
  hoy,
  iniciales,
  normalizarPlaca,
  placaValida,
} from '@shared/formato';
import { IconoAlerta, IconoCheck, IconoEntrada, IconoSalida } from '@shared/iconos';
import { AvisoError, Boton, Campo, claseEntrada, EtiquetaMovimiento, Pildora, Placa } from '@shared/ui';
import { OpcionesTurno } from './OpcionesTurno';

export interface Turno {
  caseta: Caseta;
  /** Inicio del turno, ISO. */
  desde: string;
}

interface Props {
  inicio: Inicio;
  turno: Turno;
  onTerminar: () => void;
  onCambiarCaseta: () => void;
}

const VISITA_VACIA = { tipoPersona: 'VISITANTE' as TipoPersona, nombrePersona: '', motivo: '' };

export function Trabajo({ inicio, turno, onTerminar, onCambiarCaseta }: Props) {
  const { usuario } = inicio;
  const [texto, setTexto] = useState('');
  const [verificacion, setVerificacion] = useState<VerificacionPlaca | null>(null);
  const [ocupado, setOcupado] = useState<'verificar' | 'registrar' | null>(null);
  const [error, setError] = useState<ErrorApi | null>(null);
  const [visita, setVisita] = useState(VISITA_VACIA);
  const [nota, setNota] = useState<string | null>(null);
  const [resultado, setResultado] = useState<ResultadoRegistro | null>(null);
  const [cuenta, setCuenta] = useState({ entradas: 0, salidas: 0 });
  const [menuAbierto, setMenuAbierto] = useState(false);
  /** Movimiento distinto al sugerido que espera confirmación del guardia. */
  const [confirmar, setConfirmar] = useState<TipoMovimiento | null>(null);
  const campoPlaca = useRef<HTMLInputElement>(null);

  const recientes = useConsulta('movimientos.buscar', {
    casetaId: turno.caseta.id,
    desde: hoy(),
    hasta: hoy(),
    porPagina: 4,
  });

  const placa = normalizarPlaca(texto);
  const campos = error?.campos ?? {};

  async function verificar(e: FormEvent) {
    e.preventDefault();
    if (!placaValida(placa)) {
      setError({ codigo: 'VALIDACION', mensaje: 'Revisa la placa.', campos: { placa: 'La placa debe tener entre 4 y 10 letras o números.' } });
      return;
    }
    setOcupado('verificar');
    setError(null);
    const r = await llamarApi('movimientos.verificarPlaca', { placa });
    setOcupado(null);
    if (!r.ok) return setError(r.error);
    setVerificacion(r.datos);
    setVisita(VISITA_VACIA);
    setConfirmar(null);
  }

  async function registrar(tipo: TipoMovimiento) {
    if (!verificacion) return;
    // Salida sin entrada o entrada repetida: el servidor la acepta con advertencia,
    // pero el guardia confirma antes de guardar.
    if (tipo !== verificacion.siguienteTipo && confirmar !== tipo) {
      setError(null);
      setConfirmar(tipo);
      return;
    }
    const pideVisita = necesitaDatosDeVisita(verificacion, tipo);
    if (pideVisita && !visita.nombrePersona.trim()) {
      setError({
        codigo: 'VALIDACION',
        mensaje: 'Revisa los datos de la visita.',
        campos: { nombrePersona: 'Escribe el nombre del conductor.' },
      });
      document.getElementById('nombrePersona')?.focus();
      return;
    }
    setOcupado('registrar');
    setError(null);
    const r = await llamarApi('movimientos.registrar', {
      tipo,
      placa: verificacion.placa,
      casetaId: turno.caseta.id,
      ...(pideVisita ? visita : {}),
      ...(nota?.trim() ? { observaciones: nota.trim() } : {}),
    });
    setOcupado(null);
    if (!r.ok) return setError(r.error);
    setResultado(r.datos);
    setConfirmar(null);
    setCuenta((c) => (tipo === 'ENTRADA' ? { ...c, entradas: c.entradas + 1 } : { ...c, salidas: c.salidas + 1 }));
    void recientes.recargar();
  }

  function otroVehiculo() {
    setResultado(null);
    setVerificacion(null);
    setTexto('');
    setNota(null);
    setError(null);
    setConfirmar(null);
    requestAnimationFrame(() => campoPlaca.current?.focus());
  }

  const pastilla = <Pastilla usuario={usuario} caseta={turno.caseta} onAbrir={() => setMenuAbierto(true)} />;
  const listaRecientes = (
    <Recientes
      movimientos={recientes.datos?.elementos ?? null}
      error={recientes.error}
      destacado={resultado?.movimiento.id}
      onReintentar={recientes.recargar}
    />
  );

  return (
    <div className="mx-auto flex min-h-screen max-w-md flex-col gap-4.5 bg-white px-4 pt-4 pb-6">
      {resultado ? (
        <Confirmacion resultado={resultado} pastilla={pastilla} onOtro={otroVehiculo} />
      ) : (
        <>
          <form onSubmit={verificar} className="flex flex-col gap-2" noValidate>
            <div className="flex items-center justify-between gap-3">
              <label htmlFor="placa" className="text-[15px] font-semibold">
                Placa del vehículo
              </label>
              {pastilla}
            </div>
            <div className="relative rounded-xl border-[3px] border-tinta bg-white px-3 pt-4.5 pb-2.5 shadow-[inset_0_0_0_4px_#fff,inset_0_0_0_5px_var(--color-regla)] focus-within:ring-4 focus-within:ring-amarillo">
              <span aria-hidden="true" className="absolute top-2 left-3 size-2.5 rounded-full bg-regla" />
              <span aria-hidden="true" className="absolute top-2 right-3 size-2.5 rounded-full bg-regla" />
              <input
                id="placa"
                ref={campoPlaca}
                value={texto}
                onChange={(e) => {
                  setTexto(e.target.value);
                  setError(null);
                  if (verificacion && normalizarPlaca(e.target.value) !== verificacion.placa) {
                    setVerificacion(null);
                    setConfirmar(null);
                  }
                }}
                autoComplete="off"
                autoCapitalize="characters"
                spellCheck={false}
                maxLength={12}
                placeholder="ABC-123-D"
                aria-invalid={Boolean(campos.placa)}
                aria-describedby={campos.placa ? 'placa-error' : 'placa-ayuda'}
                className="w-full border-0 bg-transparent p-0 text-center font-display text-[54px] leading-tight font-bold tracking-[5px] text-tinta uppercase placeholder:text-regla focus:outline-none"
              />
            </div>
            <div className="flex items-center justify-between gap-3">
              <span id="placa-ayuda" className="text-sm text-gris">
                {placa ? `Se guarda como ${placa}` : 'Escríbela como aparece en el vehículo.'}
              </span>
              <Boton type="submit" variante="secundario" disabled={ocupado !== null || !placa}>
                {ocupado === 'verificar' ? 'Verificando…' : 'Verificar placa'}
              </Boton>
            </div>
            {campos.placa && (
              <span id="placa-error" className="flex items-center gap-1.5 text-[15px] font-medium text-error">
                <IconoAlerta tam={18} />
                {campos.placa}
              </span>
            )}
            {!campos.placa && placaValida(placa) && !/\d/.test(placa) && (
              <span role="status" className="flex items-center gap-1.5 text-[15px] font-medium text-alerta">
                <IconoAlerta tam={18} />
                Esta placa no tiene números. Revísala antes de verificar.
              </span>
            )}
          </form>

          {error && !error.campos && <AvisoError error={error} />}

          {verificacion && (
            <>
              <ResultadoVerificacion verificacion={verificacion} />
              {necesitaDatosDeVisita(verificacion, confirmar ?? verificacion.siguienteTipo) && (
                <DatosDeVisita
                  visita={visita}
                  onCambiar={(v) => {
                    setVisita(v);
                    if (campos.nombrePersona && v.nombrePersona.trim()) setError(null);
                  }}
                  campos={campos}
                />
              )}
              <div className="flex flex-col gap-2.5">
                {confirmar ? (
                  <ConfirmarMovimiento
                    tipo={confirmar}
                    ocupado={ocupado !== null}
                    onConfirmar={() => registrar(confirmar)}
                    onCancelar={() => {
                      setConfirmar(null);
                      setError(null);
                    }}
                  />
                ) : (
                  <>
                    <BotonMovimiento
                      tipo={verificacion.siguienteTipo}
                      grande
                      disabled={ocupado !== null}
                      onClick={() => registrar(verificacion.siguienteTipo)}
                    />
                    <div className="flex gap-2.5">
                      <BotonMovimiento
                        tipo={verificacion.siguienteTipo === 'ENTRADA' ? 'SALIDA' : 'ENTRADA'}
                        disabled={ocupado !== null}
                        onClick={() => registrar(verificacion.siguienteTipo === 'ENTRADA' ? 'SALIDA' : 'ENTRADA')}
                      />
                      <Boton variante="texto" onClick={() => setNota(nota === null ? '' : null)}>
                        {nota === null ? 'Agregar nota' : 'Quitar nota'}
                      </Boton>
                    </div>
                  </>
                )}
                {nota !== null && (
                  <Campo id="nota" etiqueta="Nota para este registro" error={campos.observaciones}>
                    <textarea
                      id="nota"
                      value={nota}
                      maxLength={500}
                      rows={2}
                      onChange={(e) => setNota(e.target.value)}
                      className={claseEntrada(campos.observaciones, 'py-2')}
                    />
                  </Campo>
                )}
              </div>
            </>
          )}
        </>
      )}

      {listaRecientes}

      {menuAbierto && (
        <OpcionesTurno
          usuario={usuario}
          turno={turno}
          cuenta={cuenta}
          onCerrar={() => setMenuAbierto(false)}
          onTerminar={onTerminar}
          onCambiarCaseta={onCambiarCaseta}
        />
      )}
    </div>
  );
}

/** Placa fuera del catálogo y sin entrada abierta: hay que pedir los datos de la visita. */
function necesitaDatosDeVisita(v: VerificacionPlaca, tipo: TipoMovimiento) {
  return !v.vehiculo && !(tipo === 'SALIDA' && v.dentro);
}

function ConfirmarMovimiento({
  tipo,
  ocupado,
  onConfirmar,
  onCancelar,
}: {
  tipo: TipoMovimiento;
  ocupado: boolean;
  onConfirmar: () => void;
  onCancelar: () => void;
}) {
  const salida = tipo === 'SALIDA';
  return (
    <section role="alert" aria-labelledby="confirmar-titulo" className="flex flex-col gap-3 rounded-lg bg-alerta-suave px-4 py-3.5 text-alerta">
      <div className="flex gap-2.5">
        <IconoAlerta tam={22} />
        <div className="flex flex-col gap-0.5">
          <strong id="confirmar-titulo">{salida ? 'No hay entrada registrada' : 'Este vehículo ya está dentro'}</strong>
          <span className="text-[15px] leading-snug">
            {salida
              ? 'No encontramos una entrada de esta placa. ¿Registrar la salida de todos modos?'
              : 'Su último registro es una entrada. ¿Registrar otra entrada de todos modos?'}
          </span>
        </div>
      </div>
      <div className="flex gap-2.5">
        <Boton variante={salida ? 'salida' : 'primario'} disabled={ocupado} onClick={onConfirmar} className="min-h-13 flex-1">
          {salida ? <IconoSalida tam={20} /> : <IconoEntrada tam={20} />}
          {salida ? 'Sí, registrar salida' : 'Sí, registrar entrada'}
        </Boton>
        <Boton variante="secundario" onClick={onCancelar} className="min-h-13">
          Cancelar
        </Boton>
      </div>
    </section>
  );
}

function Pastilla({ usuario, caseta, onAbrir }: { usuario: Usuario; caseta: Caseta; onAbrir: () => void }) {
  return (
    <button
      type="button"
      onClick={onAbrir}
      aria-label={`Turno de ${usuario.nombre} en ${caseta.nombre}. Abrir opciones del turno`}
      className="btn inline-flex min-h-11 items-center gap-2 rounded-full bg-asfalto py-1 pr-1 pl-3.5 text-[15px] font-semibold text-white"
    >
      <span aria-hidden="true" className="size-2 rounded-full bg-turno" />
      {caseta.nombre}
      <span
        aria-hidden="true"
        className="flex size-[34px] items-center justify-center rounded-full bg-amarillo text-sm font-bold text-tinta"
      >
        {iniciales(usuario.nombre)}
      </span>
    </button>
  );
}

function ResultadoVerificacion({ verificacion }: { verificacion: VerificacionPlaca }) {
  const { vehiculo, persona, dentro, ultimoMovimiento: ultimo } = verificacion;
  const textoUltimo = ultimo
    ? `${ultimo.tipo === 'ENTRADA' ? 'Entró' : 'Salió'} ${cuando(ultimo.fechaHora)} por la ${ultimo.casetaNombre.toLowerCase()}.`
    : 'Es su primer registro.';

  if (!vehiculo) {
    return (
      <section role="status" className="flex gap-2.5 rounded-lg bg-alerta-suave px-3.5 py-3 text-alerta">
        <IconoAlerta tam={22} />
        <div className="flex flex-col gap-0.5">
          <strong>Placa no registrada</strong>
          <span className="text-[15px] leading-snug">
            {dentro && ultimo
              ? `${textoUltimo} ${ultimo.nombrePersona ? `Visita de ${ultimo.nombrePersona}.` : ''}`
              : 'Registra la visita con los datos del conductor.'}
          </span>
        </div>
      </section>
    );
  }

  const id = persona?.identificador
    ? `, ${persona.tipo === 'ESTUDIANTE' ? 'matrícula' : 'número de empleado'} ${persona.identificador}`
    : '';
  const auto = [vehiculo.marca, vehiculo.modelo, vehiculo.color].filter(Boolean).join(' ');

  return (
    <section aria-label="Resultado de la verificación" className="flex flex-col gap-1.5 border-t-2 border-tinta pt-4">
      <h1 className="font-display text-[32px] leading-tight font-bold">{vehiculo.nombrePersona}</h1>
      <p className="text-gris">
        {ETIQUETA_TIPO_PERSONA[vehiculo.tipoPersona]}
        {id}.{auto && ` ${auto}.`}
      </p>
      <div className="mt-1.5 flex flex-wrap gap-2">
        {vehiculo.autorizado ? (
          <Pildora tono="ok">
            <IconoCheck tam={16} />
            Autorizado
          </Pildora>
        ) : (
          <Pildora tono="error">
            <IconoAlerta tam={16} />
            No autorizado
          </Pildora>
        )}
        <Pildora tono="neutro">{dentro ? 'Dentro del campus' : 'Fuera del campus'}</Pildora>
      </div>
      <p className="mt-1 text-[15px] text-gris">{textoUltimo}</p>
    </section>
  );
}

function DatosDeVisita({
  visita,
  onCambiar,
  campos,
}: {
  visita: typeof VISITA_VACIA;
  onCambiar: (v: typeof VISITA_VACIA) => void;
  campos: Record<string, string>;
}) {
  return (
    <div className="flex flex-col gap-3.5">
      <fieldset className="flex flex-col gap-2">
        <legend className="mb-2 text-[15px] font-semibold">Tipo de usuario</legend>
        <div className="grid grid-cols-2 gap-2">
          {TIPOS_PERSONA.map((t) => {
            const activo = visita.tipoPersona === t;
            return (
              <label
                key={t}
                className={`flex min-h-12 cursor-pointer items-center gap-2.5 rounded-lg border-[1.5px] px-3 ${
                  activo ? 'border-asfalto bg-asfalto font-semibold text-white' : 'border-regla font-medium'
                }`}
              >
                <input
                  type="radio"
                  name="tipoPersona"
                  value={t}
                  checked={activo}
                  onChange={() => onCambiar({ ...visita, tipoPersona: t })}
                  className="size-4.5 accent-amarillo"
                />
                {ETIQUETA_TIPO_PERSONA[t]}
              </label>
            );
          })}
        </div>
        {campos.tipoPersona && <span className="text-[15px] font-medium text-error">{campos.tipoPersona}</span>}
      </fieldset>
      <Campo id="nombrePersona" etiqueta="Nombre del conductor" error={campos.nombrePersona}>
        <input
          id="nombrePersona"
          value={visita.nombrePersona}
          maxLength={100}
          aria-required="true"
          aria-invalid={Boolean(campos.nombrePersona)}
          onChange={(e) => onCambiar({ ...visita, nombrePersona: e.target.value })}
          className={claseEntrada(campos.nombrePersona, 'min-h-12 text-[17px]')}
        />
      </Campo>
      <Campo id="motivo" etiqueta="Motivo de la visita" error={campos.motivo}>
        <input
          id="motivo"
          value={visita.motivo}
          maxLength={200}
          onChange={(e) => onCambiar({ ...visita, motivo: e.target.value })}
          className={claseEntrada(campos.motivo, 'min-h-12 text-[17px]')}
        />
      </Campo>
    </div>
  );
}

function BotonMovimiento({
  tipo,
  grande = false,
  disabled,
  onClick,
}: {
  tipo: TipoMovimiento;
  grande?: boolean;
  disabled: boolean;
  onClick: () => void;
}) {
  const Icono = tipo === 'ENTRADA' ? IconoEntrada : IconoSalida;
  const texto = tipo === 'ENTRADA' ? 'Registrar entrada' : 'Registrar salida';

  if (grande) {
    return (
      <Boton
        variante={tipo === 'ENTRADA' ? 'primario' : 'salida'}
        disabled={disabled}
        onClick={onClick}
        className="min-h-16 font-display text-[26px] font-bold"
      >
        <Icono tam={26} />
        {texto}
      </Boton>
    );
  }
  return (
    <Boton variante="secundario" disabled={disabled} onClick={onClick} className="min-h-13 flex-1 text-[17px]">
      <span
        className={`flex size-[26px] items-center justify-center rounded-md ${
          tipo === 'ENTRADA' ? 'bg-asfalto text-white' : 'bg-amarillo text-tinta'
        }`}
      >
        <Icono tam={16} />
      </span>
      {texto}
    </Boton>
  );
}

function Confirmacion({
  resultado,
  pastilla,
  onOtro,
}: {
  resultado: ResultadoRegistro;
  pastilla: ReactNode;
  onOtro: () => void;
}) {
  const { movimiento: m, advertencias, permanenciaMinutos } = resultado;
  const persona = m.nombrePersona
    ? `${m.nombrePersona}, ${ETIQUETA_TIPO_PERSONA[m.tipoPersona].toLowerCase()}`
    : ETIQUETA_TIPO_PERSONA[m.tipoPersona];

  return (
    <>
      <div className="flex justify-end">{pastilla}</div>
      <section role="status" className="flex flex-col gap-2.5">
        <div className="flex items-center gap-3">
          <span className="flex size-13 items-center justify-center rounded-full bg-ok text-white">
            <IconoCheck tam={28} />
          </span>
          <div className="flex flex-col">
            <h1 className="font-display text-[38px] leading-none font-bold">
              {m.tipo === 'ENTRADA' ? 'Entrada registrada' : 'Salida registrada'}
            </h1>
            <span className="text-[17px] text-gris">Hoy a las {horaConSegundos(m.fechaHora)}</span>
          </div>
        </div>
        <div className="mt-1.5">
          <Placa placa={m.placa} grande />
        </div>
      </section>

      {advertencias.length > 0 && (
        <ul className="flex flex-col gap-2">
          {advertencias.map((a) => (
            <li key={a.codigo} className="flex gap-2 rounded-lg bg-alerta-suave px-3.5 py-2.5 text-[15px] text-alerta">
              <IconoAlerta tam={20} />
              {a.mensaje}
            </li>
          ))}
        </ul>
      )}

      <dl className="flex flex-col border-t-2 border-tinta">
        <Dato etiqueta="Persona" valor={persona} />
        {m.motivo && <Dato etiqueta="Motivo" valor={m.motivo} />}
        <Dato etiqueta="Caseta" valor={m.casetaNombre} />
        {permanenciaMinutos !== null && <Dato etiqueta="Estuvo dentro" valor={duracion(permanenciaMinutos)} />}
        <Dato etiqueta="Registró" valor={m.registradoPor} />
      </dl>

      <Boton onClick={onOtro} className="min-h-15 font-display text-2xl font-bold">
        Registrar otro vehículo
      </Boton>
    </>
  );
}

function Dato({ etiqueta, valor }: { etiqueta: string; valor: string }) {
  return (
    <div className="flex justify-between gap-3 border-b border-concreto py-3">
      <dt className="text-gris">{etiqueta}</dt>
      <dd className="text-right font-semibold">{valor}</dd>
    </div>
  );
}

function Recientes({
  movimientos,
  error,
  destacado,
  onReintentar,
}: {
  movimientos: ResultadoRegistro['movimiento'][] | null;
  error: ErrorApi | null;
  destacado?: string;
  onReintentar: () => void;
}) {
  return (
    <section className="flex flex-col">
      <h2 className="mb-1 text-[15px] font-semibold text-gris">Hoy en esta caseta</h2>
      {error && <AvisoError error={error} onReintentar={onReintentar} />}
      {movimientos?.length === 0 && <p className="py-3 text-gris">Todavía no hay registros hoy.</p>}
      {movimientos?.map((m) => (
        <div
          key={m.id}
          className={`-mx-2 flex min-h-13 items-center gap-3 border-b border-concreto px-2 ${m.id === destacado ? 'bg-ok-suave' : ''}`}
        >
          <span className="w-11 text-[15px] text-gris">{hora(m.fechaHora)}</span>
          <div className="flex min-w-0 flex-1 flex-col items-start gap-0.5">
            <Placa placa={m.placa} />
            <span className="truncate text-[13px] text-gris">
              {m.nombrePersona ?? 'Sin nombre'}, {ETIQUETA_TIPO_PERSONA[m.tipoPersona].toLowerCase()}
            </span>
          </div>
          <EtiquetaMovimiento tipo={m.tipo} />
        </div>
      ))}
    </section>
  );
}
