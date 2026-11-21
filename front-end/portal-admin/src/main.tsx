import '@shared/estilos.css';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BannerDemo } from '@shared/BannerDemo';
import { App } from './App';

createRoot(document.getElementById('raiz')!).render(
  <StrictMode>
    <App />
    <BannerDemo />
  </StrictMode>,
);
