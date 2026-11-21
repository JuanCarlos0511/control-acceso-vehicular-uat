/** Aviso fijo cuando la app se compiló con VITE_DEMO=true (datos del mock, sin backend real). */
export function BannerDemo() {
  if (import.meta.env.VITE_DEMO !== 'true') return null;
  return (
    <p
      role="note"
      className="pointer-events-none fixed bottom-2 left-2 z-50 m-0 rounded-full bg-amarillo px-3 py-1 text-sm font-semibold text-tinta shadow"
    >
      Demostración · datos ficticios
    </p>
  );
}
