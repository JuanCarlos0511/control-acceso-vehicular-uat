import { useState } from 'react';
import { useConsulta } from '@shared/api';
import type { Agrupacion, VehiculoFrecuente } from '@shared/contrato';
import { ETIQUETA_TIPO_PERSONA, fechaHoraCorta, hoy, numero, sumarDias } from '@shared/formato';
import { AvisoError, Campo, Cargando, claseEntrada, Pildora, Placa } from '@shared/ui';
import { BarrasDobles, BotonesExportar, EncabezadoPagina, Leyenda, Panel, SelectorCaseta, Tabla, useExportar } from '../comun';

const DIAS = ['Lun', 'Mar', 'Mié', 'Jue', 'Vie', 'Sáb', 'Dom'];
const DIAS_LARGOS = ['lunes', 'martes', 'miércoles', 'jueves', 'viernes', 'sábado', 'domingo'];
const INICIAL_DIA = 'LMXJVSD';
const MESES = ['ene', 'feb', 'mar', 'abr', 'may', 'jun', 'jul', 'ago', 'sep', 'oct', 'nov', 'dic'];
// Rampa de pintura vial a asfalto: aclara el amarillo y oscurece hacia el pico.
const RAMPA = ['#f2f3f0', '#fbe7a6', '#f5b800', '#9c8448', '#5c5a4e', '#2b3036'];

const AGRUPACIONES: Array<[Agrupacion, string]> = [
  ['DIA', 'Día'],
  ['SEMANA', 'Semana'],
  ['MES', 'Mes'],
];

function etiquetaPeriodo(periodo: string, agrupacion: Agrupacion): string {
  if (agrupacion === 'MES') return `${MESES[Number(periodo.slice(5, 7)) - 1]}\n${periodo.slice(0, 4)}`;
  const d = new Date(`${periodo}T12:00:00-06:00`).getUTCDay();
  const dia = Number(periodo.slice(8, 10));
  if (agrupacion === 'SEMANA') return `Semana\n${dia}/${Number(periodo.slice(5, 7))}`;
  return `${INICIAL_DIA[(d + 6) % 7]}\n${dia}`;
}

function colorCelda(valor: number, tope: number) {
  if (!valor) return RAMPA[0]!;
  const r = valor / tope;
  return RAMPA[r < 0.15 ? 1 : r < 0.35 ? 2 : r < 0.6 ? 3 : r < 0.85 ? 4 : 5]!;
}

export function Reportes() {
  const [desde, setDesde] = useState(sumarDias(hoy(), -13));
  const [hasta, setHasta] = useState(hoy());
  const [casetaId, setCasetaId] = useState('');
  const [agrupacion, setAgrupacion] = useState<Agrupacion>('DIA');
  const periodo = { desde, hasta, ...(casetaId && { casetaId }) };

  const resumen = useConsulta('reportes.resumen', { ...periodo, agrupacion });
  const frecuentes = useConsulta('reportes.vehiculosFrecuentes', { ...periodo, limite: 10 });
  const afluencia = useConsulta('reportes.afluencia', periodo);
  const { ocupado, error: errorExportar, exportar } = useExportar('reportes.resumen');
  const error = resumen.error ?? frecuentes.error ?? afluencia.error;

  const totalEntradas = resumen.datos?.totales.entradas ?? 0;
  const topeTipo = Math.max(1, ...(resumen.datos?.porTipoPersona.map((t) => t.entradas) ?? []));
  const topeFrecuente = Math.max(1, ...(frecuentes.datos?.map((v) => v.entradas) ?? []));
  const topeCelda = Math.max(1, ...(afluencia.datos?.matriz.flat() ?? []));
  const pico = afluencia.datos?.pico;

  function alExportar(formato: 'XLSX' | 'PDF') {
    void exportar(formato, { ...periodo, agrupacion }, `Reportes del ${desde} al ${hasta}`, `reportes-${desde}-${hasta}`, async () => {
      if (!resumen.datos || !frecuentes.datos || !afluencia.datos) {
        return { codigo: 'ERROR_INTERNO', mensaje: 'Espera a que terminen de cargar los reportes.' };
      }
      return [
        { nombre: 'Por periodo', columnas: ['Periodo', 'Entradas', 'Salidas'], filas: resumen.datos.serie.map((s) => [s.periodo, s.entradas, s.salidas]) },
        { nombre: 'Por caseta', columnas: ['Caseta', 'Entradas', 'Salidas'], filas: resumen.datos.porCaseta.map((c) => [c.casetaNombre, c.entradas, c.salidas]) },
        {
          nombre: 'Por tipo de usuario',
          columnas: ['Tipo de usuario', 'Entradas', 'Salidas'],
          filas: resumen.datos.porTipoPersona.map((t) => [ETIQUETA_TIPO_PERSONA[t.tipoPersona], t.entradas, t.salidas]),
        },
        {
          nombre: 'Vehículos frecuentes',
          columnas: ['Placa', 'Persona', 'Tipo', 'Entradas', 'Última entrada'],
          filas: frecuentes.datos.map((v) => [v.placa, v.nombrePersona ?? '', ETIQUETA_TIPO_PERSONA[v.tipoPersona], v.entradas, fechaHoraCorta(v.ultimaEntrada)]),
        },
        {
          nombre: 'Afluencia',
          columnas: ['Día', ...Array.from({ length: 24 }, (_, h) => `${h}:00`)],
          filas: afluencia.datos.matriz.map((fila, d) => [DIAS[d]!, ...fila]),
        },
      ];
    });
  }

  return (
    <>
      <EncabezadoPagina
        titulo="Reportes"
        descripcion="Entradas, salidas y afluencia en el periodo que elijas."
        acciones={
          <>
            <Campo id="r-desde" etiqueta="Desde">
              <input id="r-desde" type="date" value={desde} onChange={(e) => setDesde(e.target.value)} className={claseEntrada()} />
            </Campo>
            <Campo id="r-hasta" etiqueta="Hasta">
              <input id="r-hasta" type="date" value={hasta} onChange={(e) => setHasta(e.target.value)} className={claseEntrada()} />
            </Campo>
            <Campo id="r-caseta" etiqueta="Caseta">
              <SelectorCaseta id="r-caseta" valor={casetaId} onCambiar={setCasetaId} todas="Las tres" />
            </Campo>
            <BotonesExportar ocupado={ocupado} onExportar={alExportar} />
          </>
        }
      />
      {errorExportar && <AvisoError error={errorExportar} />}
      {error && <AvisoError error={error} />}

      <Panel
        titulo="Entradas y salidas por periodo"
        descripcion={
          resumen.datos && `${numero(resumen.datos.totales.entradas)} entradas y ${numero(resumen.datos.totales.salidas)} salidas en el periodo.`
        }
        acciones={
          <div role="group" aria-label="Agrupar por" className="flex gap-1 rounded-lg bg-concreto p-1">
            {AGRUPACIONES.map(([valor, texto]) => (
              <button
                key={valor}
                type="button"
                aria-pressed={agrupacion === valor}
                onClick={() => setAgrupacion(valor)}
                className={`min-h-10 rounded-md px-4 ${agrupacion === valor ? 'bg-asfalto font-semibold text-white' : 'font-medium text-tinta'}`}
              >
                {texto}
              </button>
            ))}
          </div>
        }
      >
        <div className="mb-3">
          <Leyenda />
        </div>
        {resumen.datos ? (
          <BarrasDobles
            datos={resumen.datos.serie.map((s) => ({ etiqueta: etiquetaPeriodo(s.periodo, agrupacion), entradas: s.entradas, salidas: s.salidas }))}
            descripcion={`Entradas y salidas por ${agrupacion === 'DIA' ? 'día' : agrupacion === 'SEMANA' ? 'semana' : 'mes'} del ${desde} al ${hasta}.`}
          />
        ) : (
          <Cargando />
        )}
      </Panel>

      <div className="flex flex-wrap gap-6">
        <Panel titulo="Por caseta" className="flex-[1_1_420px]">
          {resumen.datos ? (
            <Tabla
              minAncho={380}
              filas={resumen.datos.porCaseta}
              clave={(c) => c.casetaId}
              columnas={[
                { titulo: 'Caseta', celda: (c) => <span className="font-semibold">{c.casetaNombre}</span> },
                { titulo: 'Entradas', alinear: 'derecha', celda: (c) => numero(c.entradas) },
                { titulo: 'Salidas', alinear: 'derecha', celda: (c) => numero(c.salidas) },
                {
                  titulo: 'Del total',
                  alinear: 'derecha',
                  celda: (c) => <span className="text-gris">{totalEntradas ? Math.round((c.entradas / totalEntradas) * 100) : 0} %</span>,
                },
              ]}
            />
          ) : (
            <Cargando />
          )}
        </Panel>
        <Panel titulo="Entradas por tipo de usuario" className="flex-[1_1_420px]">
          {resumen.datos ? (
            <div className="flex flex-col gap-4">
              {resumen.datos.porTipoPersona.map((t) => (
                <div key={t.tipoPersona} className="grid grid-cols-[120px_minmax(0,1fr)_120px] items-center gap-3">
                  <span className="font-semibold">{ETIQUETA_TIPO_PERSONA[t.tipoPersona]}</span>
                  <div aria-hidden="true" className="h-3 rounded-sm bg-concreto">
                    <div className="h-full rounded-sm bg-asfalto" style={{ width: `${(t.entradas / topeTipo) * 100}%` }} />
                  </div>
                  <span className="text-right text-gris">
                    {numero(t.entradas)} ({totalEntradas ? Math.round((t.entradas / totalEntradas) * 100) : 0} %)
                  </span>
                </div>
              ))}
            </div>
          ) : (
            <Cargando />
          )}
        </Panel>
      </div>

      <Panel titulo="Vehículos más frecuentes" descripcion="Ordenados por número de entradas.">
        {frecuentes.datos ? (
          <Tabla<VehiculoFrecuente>
            minAncho={860}
            filas={frecuentes.datos}
            clave={(v) => v.placa}
            columnas={[
              { titulo: 'Lugar', celda: (v) => <span className="font-display text-xl font-bold">{frecuentes.datos!.indexOf(v) + 1}</span> },
              { titulo: 'Placa', celda: (v) => <Placa placa={v.placa} /> },
              {
                titulo: 'Persona',
                celda: (v) => (
                  <span className="flex items-center gap-2">
                    {v.nombrePersona ?? 'Sin nombre'}
                    {!v.vehiculoRegistrado && <Pildora tono="alerta">No registrada</Pildora>}
                  </span>
                ),
              },
              { titulo: 'Tipo', celda: (v) => ETIQUETA_TIPO_PERSONA[v.tipoPersona] },
              {
                titulo: 'Entradas',
                celda: (v) => (
                  <span className="flex items-center gap-2.5">
                    <span aria-hidden="true" className="h-2.5 w-36 rounded-sm bg-concreto">
                      <span className="block h-full rounded-sm bg-asfalto" style={{ width: `${(v.entradas / topeFrecuente) * 100}%` }} />
                    </span>
                    <span className="font-bold">{v.entradas}</span>
                  </span>
                ),
              },
              { titulo: 'Última entrada', celda: (v) => <span className="text-gris">{fechaHoraCorta(v.ultimaEntrada)}</span> },
            ]}
          />
        ) : (
          <Cargando />
        )}
      </Panel>

      <Panel
        titulo="Horarios de mayor afluencia"
        descripcion={
          pico
            ? `La hora con más movimiento es el ${DIAS_LARGOS[pico.dia - 1]} de ${pico.hora}:00 a ${pico.hora + 1}:00, con ${numero(pico.total)} movimientos en el periodo.`
            : 'Sin movimientos en el periodo.'
        }
      >
        {afluencia.datos ? (
          <>
            <div className="overflow-x-auto">
              <div
                role="img"
                aria-label="Mapa de calor de movimientos por día de la semana y hora."
                className="flex min-w-[780px] flex-col gap-[3px]"
              >
                {afluencia.datos.matriz.map((fila, d) => (
                  <div key={DIAS[d]} className="grid grid-cols-[48px_repeat(24,minmax(0,1fr))] items-center gap-[3px]">
                    <span className="text-sm font-semibold">{DIAS[d]}</span>
                    {fila.map((valor, h) => (
                      <span
                        key={h}
                        title={`${DIAS_LARGOS[d]} ${h}:00, ${valor} movimientos`}
                        className="h-6 rounded-sm"
                        style={{ background: colorCelda(valor, topeCelda), boxShadow: valor ? undefined : 'inset 0 0 0 1px #c9cdd2' }}
                      />
                    ))}
                  </div>
                ))}
                <div aria-hidden="true" className="mt-1 grid grid-cols-[48px_repeat(24,minmax(0,1fr))] gap-[3px]">
                  <span />
                  {Array.from({ length: 24 }, (_, h) => (
                    <span key={h} className="text-[13px] text-gris">
                      {h % 3 === 0 ? h : ''}
                    </span>
                  ))}
                </div>
              </div>
            </div>
            <div className="mt-3 flex items-center gap-1.5 text-sm text-gris">
              <span>Menos</span>
              {RAMPA.map((c) => (
                <span key={c} className="h-3 w-5 rounded-sm" style={{ background: c }} />
              ))}
              <span>Más</span>
            </div>
          </>
        ) : (
          <Cargando />
        )}
      </Panel>
    </>
  );
}
