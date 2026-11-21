import type { ReactNode, SVGProps } from 'react';

type PropsIcono = SVGProps<SVGSVGElement> & { tam?: number };

function Svg({ tam = 20, children, ...props }: PropsIcono & { children: ReactNode }) {
  return (
    <svg
      width={tam}
      height={tam}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2.25}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      className="shrink-0"
      {...props}
    >
      {children}
    </svg>
  );
}

export const IconoEntrada = (p: PropsIcono) => (
  <Svg {...p}>
    <path d="M15 4h3a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-3" />
    <path d="M10 17l5-5-5-5" />
    <path d="M15 12H3" />
  </Svg>
);

export const IconoSalida = (p: PropsIcono) => (
  <Svg {...p}>
    <path d="M9 20H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h3" />
    <path d="M16 17l5-5-5-5" />
    <path d="M21 12H9" />
  </Svg>
);

export const IconoCheck = (p: PropsIcono) => (
  <Svg {...p}>
    <path d="M5 12l5 5L20 7" />
  </Svg>
);

export const IconoAlerta = (p: PropsIcono) => (
  <Svg {...p}>
    <path d="M12 3l9.5 17h-19z" />
    <path d="M12 10v4" />
    <path d="M12 17.5v.01" />
  </Svg>
);

export const IconoInfo = (p: PropsIcono) => (
  <Svg {...p}>
    <circle cx="12" cy="12" r="9" />
    <path d="M12 11v5" />
    <path d="M12 8v.01" />
  </Svg>
);

export const IconoCerrar = (p: PropsIcono) => (
  <Svg {...p}>
    <path d="M6 6l12 12" />
    <path d="M18 6L6 18" />
  </Svg>
);

export const IconoBuscar = (p: PropsIcono) => (
  <Svg {...p}>
    <circle cx="11" cy="11" r="7" />
    <path d="M20 20l-4-4" />
  </Svg>
);

export const IconoDescarga = (p: PropsIcono) => (
  <Svg {...p}>
    <path d="M12 4v11" />
    <path d="M7 10l5 5 5-5" />
    <path d="M5 20h14" />
  </Svg>
);

export const IconoMas = (p: PropsIcono) => (
  <Svg {...p}>
    <path d="M12 5v14M5 12h14" />
  </Svg>
);

/** Pluma de caseta bajada: acceso cerrado. */
export const IconoBarrera = (p: PropsIcono) => (
  <Svg {...p}>
    <path d="M3 21V8" />
    <path d="M3 10h18v4H3" />
    <path d="M8 10l-2 4" />
    <path d="M13 10l-2 4" />
    <path d="M18 10l-2 4" />
  </Svg>
);
