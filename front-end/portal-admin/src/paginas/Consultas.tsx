import { useState, type FormEvent } from 'react';
import { llamarApi, useConsulta } from '@shared/api';
import type { FiltrosMovimientos, Movimiento, TipoMovimiento, TipoPersona } from '@shared/contrato';
import { TIPOS_PERSONA } from '@shared/contrato';
import { ETIQUETA_TIPO_PERSONA, fechaHoraCorta, hoy, numero } from '@shared/formato';
import { AvisoError, Boton, Campo, Cargando, claseEntrada, EtiquetaMovimiento, Pildora, Placa, Vacio } from '@shared/ui';
import { BotonesExportar, EncabezadoPagina, Paginador, Panel, SelectorCaseta, Tabla, useExportar } from '../comun';

const POR_PAGINA = 20;

const INICIALES = { placa: '', desde: hoy(), hasta: hoy(), casetaId: '', tipoPersona: '', tipo: '' };
type Filtros = typeof INICIALES;

function aSolicitud(f: Filtros): FiltrosMovimientos {
  return {
    ...(f.placa.trim() && { placa: f.placa.trim() }),
    ...(f.desde && { desde: f.desde }),
    ...(f.hasta && { hasta: f.hasta }),
    ...(f.casetaId && { casetaId: f.casetaId }),
    ...(f.tipoPersona && { tipoPersona: f.tipoPersona as TipoPersona }),
    ...(f.tipo && { tipo: f.tipo as TipoMovimiento }),
  };
}

function aviso(m: Movimiento) {
  if (!m.vehiculoRegistrado) return <Pildora tono="alerta">No registrada</Pildora>;
  if (!m.autorizado) return <Pildora tono="error">No autorizada</Pildora>;
  return <span className="text-gris">Ninguno</span>;
}

export const columnasMovimientos = [
  'Fecha y hora',
  'Placa',
  'Movimiento',
  'Caseta',
  'Tipo de usuario',
  'Persona',
  'Motivo',
  'Registró',
];

export function filaMovimiento(m: Movimiento) {
  return [
    fechaHoraCorta(m.fechaHora),
    m.placa,
    m.tipo === 'ENTRADA' ? 'Entrada' : 'Salida',
    m.casetaNombre,
    ETIQUETA_TIPO_PERSONA[m.tipoPersona],
    m.nombrePersona ?? '',
    m.motivo ?? '',
    m.registradoPor,
  ];
}

export function Consultas() {
  const [borrador, setBorrador] = useState<Filtros>(INICIALES);
  const [filtros, setFiltros] = useState<Filtros>(INICIALES);
  const [pagina, setPagina] = useState(1);
  const solicitud = aSolicitud(filtros);
  const res = useConsulta('movimientos.buscar', { ...solicitud, pagina, porPagina: POR_PAGINA });
  const { ocupado, error: errorExportar, exportar } = useExportar('movimientos.buscar');
  const campos = res.error?.campos ?? {};

  const cambiar = (campo: keyof Filtros) => (valor: string) => setBorrador((b) => ({ ...b, [campo]: valor }));

  function buscar(e: FormEvent) {
    e.preventDefault();
    setFiltros(borrador);
    setPagina(1);
  }

  function alExportar(formato: 'XLSX' | 'PDF') {
    void exportar(formato, solicitud, 'Movimientos registrados', `movimientos-${filtros.desde}-${filtros.hasta}`, async () => {
      const r = await llamarApi('movimientos.buscar', { ...solicitud, porPagina: 0 });
      if (!r.ok) return r.error;
      return [{ nombre: 'Movimientos', columnas: columnasMovimientos, filas: r.datos.elementos.map(filaMovimiento) }];
    });
  }

  return (
    <>
      <EncabezadoPagina
        titulo="Consultas de movimientos"
        descripcion="Busca por placa, fechas, caseta o tipo de usuario."
        acciones={<BotonesExportar ocupado={ocupado} onExportar={alExportar} />}
      />
      {errorExportar && <AvisoError error={errorExportar} />}

      <form onSubmit={buscar} aria-label="Filtros" noValidate className="flex flex-col gap-4.5 rounded-md bg-white px-6 py-5.5">
        <div className="flex flex-wrap gap-4">
          <Campo id="f-placa" etiqueta="Placa" error={campos.placa} className="flex-[1_1_180px]">
            <input id="f-placa" value={borrador.placa} onChange={(e) => cambiar('placa')(e.target.value)} placeholder="Completa o una parte" className={claseEntrada(campos.placa)} />
          </Campo>
          <Campo id="f-desde" etiqueta="Desde" error={campos.desde} className="flex-[1_1_150px]">
            <input id="f-desde" type="date" value={borrador.desde} onChange={(e) => cambiar('desde')(e.target.value)} className={claseEntrada(campos.desde)} />
          </Campo>
          <Campo id="f-hasta" etiqueta="Hasta" error={campos.hasta} className="flex-[1_1_150px]">
            <input id="f-hasta" type="date" value={borrador.hasta} onChange={(e) => cambiar('hasta')(e.target.value)} className={claseEntrada(campos.hasta)} />
          </Campo>
          <Campo id="f-caseta" etiqueta="Caseta" className="flex-[1_1_180px]">
            <SelectorCaseta id="f-caseta" valor={borrador.casetaId} onCambiar={cambiar('casetaId')} />
          </Campo>
          <Campo id="f-tipo" etiqueta="Tipo de usuario" className="flex-[1_1_170px]">
            <select id="f-tipo" value={borrador.tipoPersona} onChange={(e) => cambiar('tipoPersona')(e.target.value)} className={claseEntrada()}>
              <option value="">Todos</option>
              {TIPOS_PERSONA.map((t) => (
                <option key={t} value={t}>
                  {ETIQUETA_TIPO_PERSONA[t]}
                </option>
              ))}
            </select>
          </Campo>
          <Campo id="f-mov" etiqueta="Movimiento" className="flex-[1_1_170px]">
            <select id="f-mov" value={borrador.tipo} onChange={(e) => cambiar('tipo')(e.target.value)} className={claseEntrada()}>
              <option value="">Entradas y salidas</option>
              <option value="ENTRADA">Solo entradas</option>
              <option value="SALIDA">Solo salidas</option>
            </select>
          </Campo>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <Boton type="submit">Buscar movimientos</Boton>
          <Boton
            variante="texto"
            onClick={() => {
              setBorrador(INICIALES);
              setFiltros(INICIALES);
              setPagina(1);
            }}
          >
            Limpiar filtros
          </Boton>
        </div>
      </form>

      <Panel
        titulo={res.datos ? `${numero(res.datos.total)} movimientos` : 'Movimientos'}
        descripcion="Los más recientes primero."
      >
        {res.error && !res.error.campos && <AvisoError error={res.error} onReintentar={res.recargar} />}
        {res.cargando && !res.datos && <Cargando />}
        {res.datos && res.datos.total === 0 && <Vacio>No hay movimientos con esos filtros. Prueba con otras fechas o quita la placa.</Vacio>}
        {res.datos && res.datos.total > 0 && (
          <>
            <Tabla<Movimiento>
              minAncho={1100}
              filas={res.datos.elementos}
              clave={(m) => m.id}
              columnas={[
                { titulo: 'Fecha y hora', celda: (m) => <span className="text-gris">{fechaHoraCorta(m.fechaHora)}</span> },
                { titulo: 'Placa', celda: (m) => <Placa placa={m.placa} /> },
                { titulo: 'Movimiento', celda: (m) => <EtiquetaMovimiento tipo={m.tipo} /> },
                { titulo: 'Caseta', celda: (m) => m.casetaNombre },
                { titulo: 'Tipo de usuario', celda: (m) => ETIQUETA_TIPO_PERSONA[m.tipoPersona] },
                { titulo: 'Persona', celda: (m) => m.nombrePersona ?? 'Sin nombre' },
                { titulo: 'Registró', celda: (m) => <span className="text-[15px] text-gris">{m.registradoPor}</span> },
                { titulo: 'Aviso', celda: aviso },
              ]}
            />
            <Paginador pagina={pagina} porPagina={POR_PAGINA} total={res.datos.total} onCambiar={setPagina} />
          </>
        )}
      </Panel>
    </>
  );
}
