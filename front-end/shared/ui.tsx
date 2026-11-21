import type { ButtonHTMLAttributes, ReactNode } from 'react';
import type { ErrorApi, TipoMovimiento } from './contrato';
import { ETIQUETA_MOVIMIENTO } from './formato';
import { IconoAlerta } from './iconos';

type Variante = 'primario' | 'secundario' | 'salida' | 'texto';

const VARIANTES: Record<Variante, string> = {
  primario: 'bg-asfalto text-white',
  secundario: 'border-2 border-tinta bg-white text-tinta',
  salida: 'bg-amarillo text-tinta',
  texto: 'bg-transparent px-2 text-tinta underline decoration-amarillo decoration-[3px] underline-offset-4',
};

export function Boton({
  variante = 'primario',
  className = '',
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { variante?: Variante }) {
  return (
    <button
      type="button"
      {...props}
      className={`btn inline-flex min-h-11 items-center justify-center gap-2 rounded-lg px-4 font-semibold ${VARIANTES[variante]} ${className}`}
    />
  );
}

/** Placa con borde, como se ve en el vehículo. */
export function Placa({ placa, grande = false }: { placa: string; grande?: boolean }) {
  const tam = grande
    ? 'rounded-[10px] border-[3px] px-4 py-1 text-[34px] tracking-[4px] leading-tight'
    : 'rounded border-[1.5px] px-2 text-[17px] tracking-[1.5px] leading-normal';
  return <span className={`inline-block border-tinta bg-white font-display font-bold ${tam}`}>{placa}</span>;
}

export function EtiquetaMovimiento({ tipo }: { tipo: TipoMovimiento }) {
  const color = tipo === 'ENTRADA' ? 'bg-asfalto text-white' : 'bg-amarillo text-tinta';
  return (
    <span className={`inline-block rounded-full px-2.5 py-0.5 text-sm font-semibold whitespace-nowrap ${color}`}>
      {ETIQUETA_MOVIMIENTO[tipo]}
    </span>
  );
}

type Tono = 'ok' | 'alerta' | 'error' | 'neutro';

const TONOS: Record<Tono, string> = {
  ok: 'bg-ok-suave text-ok',
  alerta: 'bg-alerta-suave text-alerta',
  error: 'bg-error-suave text-error',
  neutro: 'bg-concreto text-tinta',
};

export function Pildora({ tono, children }: { tono: Tono; children: ReactNode }) {
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-sm font-semibold whitespace-nowrap ${TONOS[tono]}`}
    >
      {children}
    </span>
  );
}

export function claseEntrada(error?: string, extra = '') {
  const borde = error ? 'border-[2.5px] border-error-borde' : 'border-[1.5px] border-borde';
  const alto = /\bmin-h-/.test(extra) ? '' : 'min-h-11';
  const tamLetra = /\btext-\[|\btext-(xs|sm|base|lg|xl)/.test(extra) ? '' : 'text-base';
  return `${alto} ${tamLetra} rounded-md bg-white px-3 text-tinta ${borde} ${extra}`;
}

export function Campo({
  id,
  etiqueta,
  error,
  ayuda,
  className = '',
  children,
}: {
  id: string;
  etiqueta: string;
  error?: string;
  ayuda?: ReactNode;
  className?: string;
  children: ReactNode;
}) {
  return (
    <div className={`flex flex-col gap-1.5 ${className}`}>
      <label htmlFor={id} className="text-[15px] font-semibold">
        {etiqueta}
      </label>
      {children}
      {error ? (
        <span id={`${id}-error`} className="flex items-center gap-1.5 text-[15px] font-medium text-error">
          <IconoAlerta tam={18} />
          {error}
        </span>
      ) : ayuda ? (
        <span className="text-[15px] text-gris">{ayuda}</span>
      ) : null}
    </div>
  );
}

export function AvisoError({ error, onReintentar }: { error: ErrorApi; onReintentar?: () => void }) {
  return (
    <div role="alert" className="flex flex-wrap items-center gap-3 rounded-lg bg-error-suave px-4 py-3 text-error">
      <IconoAlerta tam={20} />
      <span className="flex-1 font-medium">{error.mensaje}</span>
      {onReintentar && (
        <Boton variante="secundario" onClick={onReintentar}>
          Reintentar
        </Boton>
      )}
    </div>
  );
}

export function Cargando({ texto = 'Cargando…' }: { texto?: string }) {
  return (
    <p role="status" className="py-6 text-gris">
      {texto}
    </p>
  );
}

export function Vacio({ children }: { children: ReactNode }) {
  return <p className="py-6 text-gris">{children}</p>;
}
