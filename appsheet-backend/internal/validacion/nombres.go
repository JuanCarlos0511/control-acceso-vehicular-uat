package validacion

import (
	"strings"
	"unicode"
)

var sinAcento = strings.NewReplacer(
	"Á", "A", "À", "A", "Ä", "A", "Â", "A",
	"É", "E", "È", "E", "Ë", "E", "Ê", "E",
	"Í", "I", "Ì", "I", "Ï", "I", "Î", "I",
	"Ó", "O", "Ò", "O", "Ö", "O", "Ô", "O",
	"Ú", "U", "Ù", "U", "Ü", "U", "Û", "U",
	"Ñ", "N", "Ç", "C",
)

// Partículas que no distinguen a una persona y que el OCR o la MRZ omiten a menudo.
var particulas = map[string]bool{"DE": true, "DEL": true, "LA": true, "LAS": true, "LOS": true, "Y": true, "E": true}

// TokensNombre normaliza un nombre: mayúsculas, sin acentos, solo letras.
func TokensNombre(nombre string) []string {
	n := sinAcento.Replace(strings.ToUpper(nombre))
	campos := strings.FieldsFunc(n, func(r rune) bool { return !unicode.IsLetter(r) })
	tokens := campos[:0]
	for _, c := range campos {
		if !particulas[c] {
			tokens = append(tokens, c)
		}
	}
	return tokens
}

// Cobertura compara dos nombres sin importar el orden de las palabras.
// porReferencia es la fracción de palabras de ref que aparecen en doc;
// porDocumento, la fracción de palabras de doc que aparecen en ref. Cada
// palabra cuenta según su parecido (tolerando 1–2 letras mal leídas), y una
// palabra truncada por la MRZ cuenta si es prefijo de la otra.
func Cobertura(ref, doc string) (porReferencia, porDocumento float64) {
	a, b := TokensNombre(ref), TokensNombre(doc)
	if len(a) == 0 || len(b) == 0 {
		return 0, 0
	}
	return cubrir(a, b), cubrir(b, a)
}

func cubrir(de, en []string) float64 {
	usados := make([]bool, len(en))
	total := 0.0
	for _, t := range de {
		mejor, idx := 0.0, -1
		for j, u := range en {
			if usados[j] {
				continue
			}
			if s := parecido(t, u); s > mejor {
				mejor, idx = s, j
			}
		}
		if mejor >= 0.8 && idx >= 0 {
			usados[idx] = true
			total += mejor
		}
	}
	return total / float64(len(de))
}

func parecido(a, b string) float64 {
	if a == b {
		return 1
	}
	ra, rb := []rune(a), []rune(b)
	corta, larga := ra, rb
	if len(corta) > len(larga) {
		corta, larga = larga, corta
	}
	if len(corta) >= 4 && strings.HasPrefix(string(larga), string(corta)) {
		return 0.9
	}
	d := levenshtein(ra, rb)
	return 1 - float64(d)/float64(len(larga))
}

func levenshtein(a, b []rune) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			costo := 1
			if a[i-1] == b[j-1] {
				costo = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+costo)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// Distancia es la distancia de edición entre dos textos (para placas y NIV).
func Distancia(a, b string) int { return levenshtein([]rune(a), []rune(b)) }
