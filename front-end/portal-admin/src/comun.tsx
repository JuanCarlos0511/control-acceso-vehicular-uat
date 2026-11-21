import { useEffect, useState, type FormEvent, type ReactNode } from 'react';
import { llamarApi } from '@shared/api';
import type { ErrorApi } from '@shared/contrato';
import { exportarExcel, exportarPdf, type Hoja } from '@shared/exportar';
import { numero } from '@shared/formato';
import { IconoDescarga } from '@shared/iconos';
import { AvisoError, Boton, claseEntrada } from '@shared/ui';
import { useSesion } from './sesion';

export function EncabezadoPagina({ titulo, descripcion, acciones }: { titulo: string; descripcion: string; acciones?: ReactNode }) {
  return (
    <div className="flex flex-wrap items-end justify-between gap-4">
      <div>
        <h1 className="font-display text-[44px] leading-none font-bold">{titulo}</h1>
        <p className="mt-1.5 text-[17px] text-gris">{descripcion}</p>
      </div>
      {acciones && <div className="flex flex-wrap items-end gap-2.5">{acciones}</div>}
    </div>
  );
}

export function Panel({
  titulo,
  descripcion,
  acciones,
  children,
  className = '',
}: {
  titulo?: string;
  descripcion?: ReactNode;
  acciones?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={`flex min-w-0 flex-col rounded-md bg-white px-6 py-5.5 ${className}`}>
      {(titulo || acciones) && (
        <div className="mb-3 flex flex-wrap items-end justify-between gap-3">
          <div>
            {titulo && <h2 className="font-display text-[28px] leading-tight font-bold">{titulo}</h2>}
            {descripcion && <p className="mt-1 text-[15px] text-gris">{descripcion}</p>}
          </div>
          {acciones}
        </div>
      )}
      {children}
    </section>
  );
}

export interface Columna<T> {
  titulo: string;
  celda: (fila: T) => ReactNode;
  alinear?: 'derecha';
}

export function Tabla<T>({ columnas, filas, clave, minAncho = 700, destacar }: {
  columnas: Columna<T>[];
  filas: T[];
  clave: (fila: T, indice: number) => string;
  minAncho?: number;
  destacar?: (fila: T) => boolean;
}) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-left" style={{ minWidth: minAncho }}>
        <thead>
          <tr className="border-b-2 border-tinta">
            {columnas.map((c) => (
              <th key={c.titulo} scope="col" className={`py-2.5 pr-3.5 text-sm font-semibold text-gris ${c.alinear ? 'text-right' : ''}`}>
                {c.titulo}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {filas.map((f, i) => (
            <tr key={clave(f, i)} className={`border-b border-concreto ${destacar?.(f) ? 'bg-ok-suave' : ''}`}>
              {columnas.map((c) => (
                <td key={c.titulo} className={`h-12 pr-3.5 ${c.alinear ? 'text-right' : ''}`}>
                  {c.celda(f)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function Paginador({ pagina, porPagina, total, onCambiar }: { pagina: number; porPagina: number; total: number; onCambiar: (p: number) => void }) {
  const paginas = Math.max(1, Math.ceil(total / porPagina));
  const desde = total ? (pagina - 1) * porPagina + 1 : 0;
  const hasta = Math.min(pagina * porPagina, total);
  return (
    <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
      <span className="text-[15px] text-gris">
        Mostrando {numero(desde)} a {numero(hasta)} de {numero(total)}
      </span>
      <nav aria-label="Paginación" className="flex items-center gap-1.5">
        <Boton variante="secundario" disabled={pagina <= 1} onClick={() => onCambiar(pagina - 1)} aria-label="Página anterior">
          ‹
        </Boton>
        <span className="px-2 text-[15px]">
          Página {numero(pagina)} de {numero(paginas)}
        </span>
        <Boton variante="secundario" disabled={pagina >= paginas} onClick={() => onCambiar(pagina + 1)} aria-label="Página siguiente">
          ›
        </Boton>
      </nav>
    </div>
  );
}

export function SelectorCaseta({ id, valor, onCambiar, todas = 'Las tres casetas' }: { id: string; valor: string; onCambiar: (v: string) => void; todas?: string }) {
  const { casetas } = useSesion();
  return (
    <select id={id} value={valor} onChange={(e) => onCambiar(e.target.value)} className={claseEntrada()}>
      <option value="">{todas}</option>
      {casetas.map((c) => (
        <option key={c.id} value={c.id}>
          {c.ubicacion ? `${c.nombre}, ${c.ubicacion.toLowerCase()}` : c.nombre}
        </option>
      ))}
    </select>
  );
}

/** Genera el archivo en el navegador y lo deja en la bitácora (contrato §7.6). */
export function useExportar(origen: string) {
  const [ocupado, setOcupado] = useState<'XLSX' | 'PDF' | null>(null);
  const [error, setError] = useState<ErrorApi | null>(null);

  async function exportar(formato: 'XLSX' | 'PDF', filtros: object, titulo: string, archivo: string, obtenerHojas: () => Promise<Hoja[] | ErrorApi>) {
    setOcupado(formato);
    setError(null);
    try {
      const hojas = await obtenerHojas();
      if (!Array.isArray(hojas)) return setError(hojas);
      if (formato === 'XLSX') await exportarExcel(archivo, hojas);
      else await exportarPdf(titulo, archivo, hojas);
      void llamarApi('bitacora.registrarExportacion', { origen, formato, filtros });
    } catch {
      setError({ codigo: 'RED', mensaje: 'No se pudo generar el archivo. Revisa tu conexión e intenta de nuevo.' });
    } finally {
      setOcupado(null);
    }
  }

  return { ocupado, error, exportar };
}

export function BotonesExportar({ ocupado, onExportar }: { ocupado: 'XLSX' | 'PDF' | null; onExportar: (f: 'XLSX' | 'PDF') => void }) {
  return (
    <>
      <Boton variante="secundario" disabled={ocupado !== null} onClick={() => onExportar('XLSX')}>
        <IconoDescarga tam={18} />
        {ocupado === 'XLSX' ? 'Generando…' : 'Exportar a Excel'}
      </Boton>
      <Boton variante="secundario" disabled={ocupado !== null} onClick={() => onExportar('PDF')}>
        <IconoDescarga tam={18} />
        {ocupado === 'PDF' ? 'Generando…' : 'Exportar a PDF'}
      </Boton>
    </>
  );
}

export function useRetrasado<T>(valor: T, ms = 300): T {
  const [retrasado, setRetrasado] = useState(valor);
  useEffect(() => {
    const t = setTimeout(() => setRetrasado(valor), ms);
    return () => clearTimeout(t);
  }, [valor, ms]);
  return retrasado;
}

/** Panel lateral con un formulario de alta o edición. */
export function FormularioLateral({
  titulo,
  textoGuardar,
  guardando,
  error,
  onGuardar,
  onCancelar,
  children,
}: {
  titulo: string;
  textoGuardar: string;
  guardando: boolean;
  error: ErrorApi | null;
  onGuardar: () => void;
  onCancelar: () => void;
  children: ReactNode;
}) {
  function enviar(e: FormEvent) {
    e.preventDefault();
    onGuardar();
  }
  return (
    <form onSubmit={enviar} noValidate aria-label={titulo} className="flex min-w-0 flex-[1_1_340px] flex-col gap-4.5 self-start rounded-md bg-white px-6 pt-5.5 pb-6">
      <h2 className="font-display text-[28px] leading-tight font-bold">{titulo}</h2>
      {error && !error.campos && <AvisoError error={error} />}
      {children}
      <div className="flex gap-2.5">
        <Boton type="submit" className="min-h-12 flex-1" disabled={guardando}>
          {guardando ? 'Guardando…' : textoGuardar}
        </Boton>
        <Boton variante="secundario" className="min-h-12 flex-1" onClick={onCancelar}>
          Cancelar
        </Boton>
      </div>
    </form>
  );
}

export function Casilla({ etiqueta, ayuda, marcada, onCambiar }: { etiqueta: string; ayuda?: string; marcada: boolean; onCambiar: (v: boolean) => void }) {
  return (
    <label className="flex cursor-pointer items-start gap-3 border-y border-concreto py-3.5">
      <input type="checkbox" checked={marcada} onChange={(e) => onCambiar(e.target.checked)} className="mt-0.5 size-5.5 accent-asfalto" />
      <span className="flex flex-col gap-0.5">
        <span className="font-semibold">{etiqueta}</span>
        {ayuda && <span className="text-[15px] text-gris">{ayuda}</span>}
      </span>
    </label>
  );
}

/** Barras dobles (entradas en asfalto, salidas en amarillo) para series cortas. */
export function BarrasDobles({ datos, descripcion, alto = 210 }: { datos: Array<{ etiqueta: string; entradas: number; salidas: number }>; descripcion: string; alto?: number }) {
  const tope = Math.max(1, ...datos.flatMap((d) => [d.entradas, d.salidas]));
  const escala = (n: number) => Math.round((n / tope) * (alto - 10));
  return (
    <div>
      <div role="img" aria-label={descripcion} className="flex items-end gap-2 border-b-2 border-tinta" style={{ height: alto }}>
        {datos.map((d) => (
          <div key={d.etiqueta} className="flex h-full min-w-0 flex-1 items-end justify-center gap-0.5" title={`${d.etiqueta.replace('\n', ' ')}: ${d.entradas} entradas, ${d.salidas} salidas`}>
            <div className="w-[46%] max-w-5 rounded-t-sm bg-asfalto" style={{ height: escala(d.entradas) }} />
            <div className="w-[46%] max-w-5 rounded-t-sm border border-b-0 border-tinta bg-amarillo" style={{ height: escala(d.salidas) }} />
          </div>
        ))}
      </div>
      <div aria-hidden="true" className="mt-1.5 flex gap-2">
        {datos.map((d) => (
          <div key={d.etiqueta} className="min-w-0 flex-1 text-center text-[13px] leading-snug whitespace-pre-line text-gris">
            {d.etiqueta}
          </div>
        ))}
      </div>
    </div>
  );
}

export function Leyenda() {
  return (
    <div className="flex gap-4.5 text-[15px]">
      <span className="flex items-center gap-1.5">
        <span className="size-3 rounded-sm bg-asfalto" />
        Entradas
      </span>
      <span className="flex items-center gap-1.5">
        <span className="size-3 rounded-sm border border-tinta bg-amarillo" />
        Salidas
      </span>
    </div>
  );
}
