import { useId, useState, type FormEvent } from 'react';
import { IconoAlerta, IconoCheck, IconoInfo } from '@shared/iconos';
import { Boton, Campo, Pildora, claseEntrada } from '@shared/ui';
import { verificarAlumno, type ErrorRegistro, type ResultadoRegistro } from './api';

type Estado =
  | { fase: 'formulario'; error?: ErrorRegistro }
  | { fase: 'enviando' }
  | { fase: 'listo'; resultado: ResultadoRegistro };

export function App() {
  const [estado, setEstado] = useState<Estado>({ fase: 'formulario' });

  return (
    <div className="min-h-dvh">
      <header className="bg-asfalto text-white">
        <div className="mx-auto flex max-w-xl items-center gap-3 px-4 py-4">
          <span aria-hidden="true" className="h-8 w-2 rounded-sm bg-amarillo" />
          <div>
            <p className="font-display text-sm font-semibold tracking-[2px] text-concreto uppercase">Campus Sur · Control vehicular</p>
            <h1 className="font-display text-2xl leading-tight font-bold">Registro vehicular de alumnos</h1>
          </div>
        </div>
      </header>

      <main className="mx-auto flex max-w-xl flex-col gap-4 px-4 py-6">
        {estado.fase === 'listo' ? (
          <Resultado resultado={estado.resultado} onOtraCuenta={() => setEstado({ fase: 'formulario' })} />
        ) : (
          <Formulario
            enviando={estado.fase === 'enviando'}
            error={estado.fase === 'formulario' ? estado.error : undefined}
            onEnviar={async (usuario, contrasena) => {
              setEstado({ fase: 'enviando' });
              const r = await verificarAlumno(usuario, contrasena);
              setEstado(r.ok ? { fase: 'listo', resultado: r.datos } : { fase: 'formulario', error: r.error });
            }}
          />
        )}

        <Pasos />

        <footer className="px-1 pb-4 text-[14px] leading-relaxed text-gris">
          Proyecto del Hackatón de Ingeniería en Sistemas Computacionales, Facultad de Ingeniería Tampico. No es un sitio
          oficial de la Universidad Autónoma de Tamaulipas.
        </footer>
      </main>
    </div>
  );
}

function Formulario({
  enviando,
  error,
  onEnviar,
}: {
  enviando: boolean;
  error?: ErrorRegistro;
  onEnviar: (usuario: string, contrasena: string) => void;
}) {
  const id = useId();
  const [usuario, setUsuario] = useState('');
  const [contrasena, setContrasena] = useState('');
  const [ver, setVer] = useState(false);
  const [faltan, setFaltan] = useState<{ usuario?: string; contrasena?: string }>({});

  const enviar = (e: FormEvent) => {
    e.preventDefault();
    const f = {
      usuario: usuario.trim() ? undefined : 'Escribe tu correo institucional o tu matrícula.',
      contrasena: contrasena ? undefined : 'Escribe tu contraseña.',
    };
    setFaltan(f);
    if (f.usuario || f.contrasena) return;
    onEnviar(usuario.trim(), contrasena);
    setContrasena('');
  };

  return (
    <section className="rounded-xl bg-white p-5 shadow-[0_1px_0_rgb(27_31_36/0.08)] sm:p-6">
      <h2 className="font-display text-[26px] leading-tight font-bold">Confirma que eres alumno</h2>
      <p className="mt-2 text-[16px] leading-relaxed text-gris">
        Inicia sesión con la cuenta con la que entras al portal de alumnos de la UAT. La usamos una sola vez para
        confirmar que estás inscrito y para tomar tu nombre oficial, que luego se coteja con tu INE.
      </p>

      <form className="mt-5 flex flex-col gap-4" onSubmit={enviar} noValidate>
        <Campo id={`${id}-usuario`} etiqueta="Correo institucional o matrícula" error={faltan.usuario} ayuda="Por ejemplo, a2100000001@alumnos.uat.edu.mx">
          <input
            id={`${id}-usuario`}
            name="username"
            type="text"
            inputMode="email"
            autoComplete="username"
            autoCapitalize="none"
            spellCheck={false}
            value={usuario}
            disabled={enviando}
            onChange={(e) => setUsuario(e.target.value)}
            aria-invalid={!!faltan.usuario}
            aria-describedby={faltan.usuario ? `${id}-usuario-error` : undefined}
            className={claseEntrada(faltan.usuario, 'w-full')}
          />
        </Campo>

        <Campo id={`${id}-contrasena`} etiqueta="Contraseña institucional" error={faltan.contrasena}>
          <div className="flex gap-2">
            <input
              id={`${id}-contrasena`}
              name="password"
              type={ver ? 'text' : 'password'}
              autoComplete="current-password"
              value={contrasena}
              disabled={enviando}
              onChange={(e) => setContrasena(e.target.value)}
              aria-invalid={!!faltan.contrasena}
              aria-describedby={faltan.contrasena ? `${id}-contrasena-error` : undefined}
              className={claseEntrada(faltan.contrasena, 'w-full flex-1')}
            />
            <Boton variante="secundario" onClick={() => setVer((v) => !v)} aria-pressed={ver} disabled={enviando}>
              {ver ? 'Ocultar' : 'Mostrar'}
            </Boton>
          </div>
        </Campo>

        {error && (
          <div role="alert" className="flex items-start gap-3 rounded-lg bg-error-suave px-4 py-3 text-error">
            <IconoAlerta tam={20} className="mt-0.5 shrink-0" />
            <span className="font-medium">{error.mensaje}</span>
          </div>
        )}

        <Boton type="submit" disabled={enviando} className="min-h-12 text-[17px]">
          {enviando ? 'Consultando el portal de la UAT…' : 'Verificar mi cuenta'}
        </Boton>
        {enviando && (
          <p role="status" className="text-center text-[15px] text-gris">
            Puede tardar hasta medio minuto.
          </p>
        )}
      </form>

      <p className="mt-5 flex gap-2 rounded-lg bg-concreto px-3 py-2.5 text-[14px] leading-relaxed text-tinta">
        <IconoInfo tam={18} className="mt-0.5 shrink-0" />
        <span>
          Tu contraseña viaja cifrada al portal de la UAT solo para iniciar sesión. No la guardamos ni la vemos. Si
          prefieres no escribirla aquí, acude a Control Vehicular con tu credencial.
        </span>
      </p>
    </section>
  );
}

function Resultado({ resultado: r, onOtraCuenta }: { resultado: ResultadoRegistro; onOtraCuenta: () => void }) {
  const listo = r.activo && r.guardado;
  return (
    <section className="rounded-xl bg-white p-5 shadow-[0_1px_0_rgb(27_31_36/0.08)] sm:p-6" aria-live="polite">
      <div className="flex items-center gap-2">
        {r.activo ? (
          <Pildora tono="ok">
            <IconoCheck tam={16} />
            Alumno inscrito
          </Pildora>
        ) : (
          <Pildora tono="alerta">
            <IconoAlerta tam={16} />
            {r.situacion ? 'No inscrito' : 'Sin confirmar'}
          </Pildora>
        )}
      </div>

      <h2 className="mt-3 font-display text-[28px] leading-tight font-bold">{r.nombre || r.correo}</h2>

      <dl className="mt-4 grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-[16px]">
        <dt className="font-semibold text-gris">Matrícula</dt>
        <dd className="break-all">{r.matricula || '—'}</dd>
        <dt className="font-semibold text-gris">Programa</dt>
        <dd>{r.programa || '—'}</dd>
        <dt className="font-semibold text-gris">Situación</dt>
        <dd>{r.situacion || '—'}</dd>
        <dt className="font-semibold text-gris">Cuenta</dt>
        <dd className="break-all">{r.correo}</dd>
      </dl>

      <p className={`mt-4 rounded-lg px-4 py-3 font-medium ${listo ? 'bg-ok-suave text-ok' : 'bg-alerta-suave text-alerta'}`}>
        {r.mensaje}
      </p>

      <div className="mt-5 flex flex-wrap gap-3">
        {listo && r.appUrl && (
          <a href={r.appUrl} className="btn inline-flex min-h-12 items-center justify-center rounded-lg bg-amarillo px-5 text-[17px] font-semibold text-tinta no-underline">
            Abrir la app y registrar mi vehículo
          </a>
        )}
        <Boton variante="texto" onClick={onOtraCuenta}>
          Usar otra cuenta
        </Boton>
      </div>
    </section>
  );
}

function Pasos() {
  return (
    <section className="rounded-xl bg-white p-5 sm:p-6">
      <h2 className="font-display text-xl font-bold">Cómo funciona</h2>
      <ol className="mt-3 flex list-none flex-col gap-3 p-0">
        {[
          ['Confirma tu cuenta', 'Inicias sesión aquí una vez. La UAT nos dice si estás inscrito y tu nombre.'],
          ['Registra tu vehículo', 'En la app de AppSheet, con tu cuenta institucional, subes tu INE y tu tarjeta de circulación.'],
          ['Recibe tu pase', 'Revisamos los documentos (y el REPUVE) y te llega un pase con código QR para la caseta.'],
        ].map(([titulo, texto], i) => (
          <li key={titulo} className="flex gap-3">
            <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-asfalto font-display text-lg font-bold text-white">
              {i + 1}
            </span>
            <div>
              <p className="font-semibold">{titulo}</p>
              <p className="text-[15px] leading-relaxed text-gris">{texto}</p>
            </div>
          </li>
        ))}
      </ol>
    </section>
  );
}
