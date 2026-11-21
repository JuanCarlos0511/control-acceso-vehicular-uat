import { createContext, useContext } from 'react';
import type { Accion, Inicio } from '@shared/contrato';

export const SesionContext = createContext<Inicio | null>(null);

/** Datos de app.iniciar y `puede(accion)` según los permisos que mandó el servidor. */
export function useSesion() {
  const sesion = useContext(SesionContext);
  if (!sesion) throw new Error('useSesion debe usarse dentro de SesionContext');
  return { ...sesion, puede: (accion: Accion) => sesion.permisos.includes(accion) };
}
