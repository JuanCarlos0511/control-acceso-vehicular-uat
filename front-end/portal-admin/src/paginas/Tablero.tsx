import { useState } from 'react';
import { useConsulta } from '@shared/api';
import type { Movimiento } from '@shared/contrato';
import { duracion, ETIQUETA_TIPO_PERSONA, fechaLarga, hora, hoy, numero } from '@shared/formato';
import { AvisoError, Boton, Cargando, EtiquetaMovimiento, Placa } from '@shared/ui';
import { BarrasDobles, EncabezadoPagina, Leyenda, Panel, SelectorCaseta, Tabla } from '../comun';

const DOCE_HORAS = 12 * 60;

export function Tablero() {
  const [casetaId, setCasetaId] = useState('');
  const dia = hoy();
  const filtro = casetaId ? { casetaId } : {};
  const resumen = useConsulta('reportes.resumen', { desde: dia, hasta: dia, ...filtro });
  const afluencia = useConsulta('reportes.afluencia', { desde: dia, hasta: dia, ...filtro });
  const ultimos = useConsulta('movimientos.buscar', { desde: dia, hasta: dia, porPagina: 7, ...filtro });
  const dentro = useConsulta('movimientos.dentro', filtro);

  const actualizar = () => {
    void resumen.recargar();
    void afluencia.recargar();
    void ultimos.recargar();
    void dentro.recargar();
  };

  const visitas = resumen.datos?.porTipoPersona.find((t) => t.tipoPersona === 'VISITANTE')?.entradas ?? 0;
  const horas = afluencia.datos?.porHora.filter((h) => h.hora >= 6 && h.hora <= 22) ?? [];
  const tope = Math.max(1, ...(resumen.datos?.porCaseta.map((c) => Math.max(c.entradas, c.salidas)) ?? []));
  const error = resumen.error ?? afluencia.error ?? ultimos.error ?? dentro.error;

  return (
    <>
      <EncabezadoPagina
        titulo="Hoy en el campus"
        descripcion={fechaLarga()}
        acciones={
          <>
            <label htmlFor="t-caseta" className="self-center text-[15px] font-semibold">
              Caseta
            </label>
            <SelectorCaseta id="t-caseta" valor={casetaId} onCambiar={setCasetaId} />
            <Boton variante="secundario" onClick={actualizar}>
              Actualizar datos
            </Boton>
          </>
        }
      />

      {error && <AvisoError error={error} onReintentar={actualizar} />}

      <section aria-label="Totales de hoy" className="flex flex-wrap rounded-md bg-white">
        <Indicador valor={resumen.datos?.totales.entradas} etiqueta="entradas" muestra="bg-asfalto" />
        <Indicador valor={resumen.datos?.totales.salidas} etiqueta="salidas" muestra="border border-tinta bg-amarillo" />
        <Indicador valor={dentro.datos?.length} etiqueta="vehículos dentro ahora" />
        <Indicador valor={resumen.datos ? visitas : undefined} etiqueta="entradas de visita" />
      </section>

      <div className="flex flex-wrap gap-6">
        <Panel titulo="Afluencia por hora" acciones={<Leyenda />} className="flex-[2_1_560px]">
          {afluencia.datos ? (
            <BarrasDobles
              datos={horas.map((h) => ({ etiqueta: String(h.hora), entradas: h.entradas, salidas: h.salidas }))}
              descripcion={`Entradas y salidas por hora de hoy, de 6 a 22 h.${afluencia.datos.pico ? ` La hora con más movimiento fue a las ${afluencia.datos.pico.hora}:00.` : ''}`}
            />
          ) : (
            <Cargando />
          )}
        </Panel>

        <Panel titulo="Por caseta" className="flex-[1_1_320px]">
          {resumen.datos ? (
            <div className="flex flex-col gap-4.5">
              {resumen.datos.porCaseta.map((c) => (
                <div key={c.casetaId} className="flex flex-col gap-1.5">
                  <div className="flex flex-wrap justify-between gap-x-3 gap-y-1">
                    <span className="font-semibold">{c.casetaNombre}</span>
                    <span className="text-gris">
                      {numero(c.entradas)} entradas, {numero(c.salidas)} salidas
                    </span>
                  </div>
                  <div aria-hidden="true" className="flex flex-col gap-0.5">
                    <div className="h-2 rounded-sm bg-asfalto" style={{ width: `${(c.entradas / tope) * 100}%` }} />
                    <div className="h-2 rounded-sm border border-tinta bg-amarillo" style={{ width: `${(c.salidas / tope) * 100}%` }} />
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <Cargando />
          )}
        </Panel>
      </div>

      <div className="flex flex-wrap items-start gap-6">
        <Panel
          titulo="Últimos movimientos"
          acciones={
            <a href="#consultas" className="font-semibold text-tinta">
              Ver todos los movimientos
            </a>
          }
          className="flex-[2_1_560px]"
        >
          {ultimos.datos ? (
            <Tabla<Movimiento>
              filas={ultimos.datos.elementos}
              clave={(m) => m.id}
              columnas={[
                { titulo: 'Hora', celda: (m) => <span className="text-gris">{hora(m.fechaHora)}</span> },
                { titulo: 'Placa', celda: (m) => <Placa placa={m.placa} /> },
                { titulo: 'Movimiento', celda: (m) => <EtiquetaMovimiento tipo={m.tipo} /> },
                { titulo: 'Caseta', celda: (m) => m.casetaNombre },
                { titulo: 'Tipo de usuario', celda: (m) => ETIQUETA_TIPO_PERSONA[m.tipoPersona] },
                { titulo: 'Persona', celda: (m) => m.nombrePersona ?? 'Sin nombre' },
              ]}
            />
          ) : (
            <Cargando />
          )}
        </Panel>

        <Panel
          titulo="Más tiempo dentro"
          descripcion="Más de 12 horas puede ser una salida que no se registró."
          className="flex-[1_1_320px]"
        >
          {dentro.datos ? (
            dentro.datos.slice(0, 5).map(({ entrada, permanenciaMinutos }) => (
              <div key={entrada.id} className="flex min-h-14 items-center gap-3 border-t border-concreto">
                <div className="flex min-w-0 flex-1 flex-col items-start gap-0.5">
                  <Placa placa={entrada.placa} />
                  <span className="truncate text-sm text-gris">
                    {entrada.nombrePersona ?? 'Sin nombre'}, entró a las {hora(entrada.fechaHora)}
                  </span>
                </div>
                <span
                  className={`rounded-full px-2.5 py-0.5 text-sm font-semibold whitespace-nowrap ${
                    permanenciaMinutos > DOCE_HORAS ? 'bg-alerta-suave text-alerta' : 'bg-concreto'
                  }`}
                >
                  {duracion(permanenciaMinutos)}
                </span>
              </div>
            ))
          ) : (
            <Cargando />
          )}
        </Panel>
      </div>
    </>
  );
}

function Indicador({ valor, etiqueta, muestra }: { valor: number | undefined; etiqueta: string; muestra?: string }) {
  return (
    <div className="flex flex-[1_1_220px] flex-col gap-1 border-concreto px-6 py-5 not-first:border-t sm:not-first:border-t-0 sm:not-first:border-l">
      <span className="font-display text-6xl leading-none font-bold">{valor === undefined ? '…' : numero(valor)}</span>
      <span className="flex items-center gap-2 text-[17px] text-gris">
        {muestra && <span aria-hidden="true" className={`size-3 rounded-sm ${muestra}`} />}
        {etiqueta}
      </span>
    </div>
  );
}
