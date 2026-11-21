// Exportación a Excel y PDF en el navegador (contrato §10). Las librerías se cargan del CDN
// solo al exportar, para no inflar el HTML que sirve Apps Script.

declare global {
  interface Window {
    XLSX?: any;
    jspdf?: any;
  }
}

const CDN = {
  xlsx: 'https://cdn.jsdelivr.net/npm/xlsx@0.18.5/dist/xlsx.full.min.js',
  jspdf: 'https://cdn.jsdelivr.net/npm/jspdf@2.5.1/dist/jspdf.umd.min.js',
  autotable: 'https://cdn.jsdelivr.net/npm/jspdf-autotable@3.8.2/dist/jspdf.plugin.autotable.min.js',
};

const cargados = new Map<string, Promise<void>>();

function cargarScript(url: string): Promise<void> {
  let promesa = cargados.get(url);
  if (!promesa) {
    promesa = new Promise<void>((resolve, reject) => {
      const s = document.createElement('script');
      s.src = url;
      s.onload = () => resolve();
      s.onerror = () => {
        cargados.delete(url);
        reject(new Error(`No se pudo cargar ${url}`));
      };
      document.head.appendChild(s);
    });
    cargados.set(url, promesa);
  }
  return promesa;
}

export interface Hoja {
  nombre: string;
  columnas: string[];
  filas: Array<Array<string | number>>;
}

export async function exportarExcel(archivo: string, hojas: Hoja[]): Promise<void> {
  await cargarScript(CDN.xlsx);
  const XLSX = window.XLSX;
  const libro = XLSX.utils.book_new();
  for (const h of hojas) {
    const hoja = XLSX.utils.aoa_to_sheet([h.columnas, ...h.filas]);
    XLSX.utils.book_append_sheet(libro, hoja, h.nombre.slice(0, 31));
  }
  XLSX.writeFile(libro, `${archivo}.xlsx`);
}

export async function exportarPdf(titulo: string, archivo: string, hojas: Hoja[]): Promise<void> {
  await cargarScript(CDN.jspdf);
  await cargarScript(CDN.autotable);
  const doc = new window.jspdf.jsPDF({ orientation: 'landscape' });
  doc.setFontSize(16);
  doc.text(titulo, 14, 16);
  hojas.forEach((h, i) => {
    let y = 26;
    if (i > 0) {
      doc.addPage();
      y = 16;
    }
    doc.setFontSize(12);
    doc.text(h.nombre, 14, y);
    doc.autoTable({
      head: [h.columnas],
      body: h.filas,
      startY: y + 4,
      styles: { fontSize: 9 },
      headStyles: { fillColor: [43, 48, 54] },
    });
  });
  doc.save(`${archivo}.pdf`);
}
