package rag

import (
	"math"
	"testing"
)

func TestEmbedTextLengthAndNormalization(t *testing.T) {
	v := EmbedText("estação de trem em Tóquio")
	if len(v) != VectorDim {
		t.Fatalf("esperado vetor de dimensão %d, obtido %d", VectorDim, len(v))
	}

	var sumSq float64
	for _, val := range v {
		sumSq += float64(val * val)
	}

	norm := math.Sqrt(sumSq)
	if math.Abs(norm-1.0) > 1e-4 {
		t.Errorf("vetor não normalizado: norma esperada ~1.0, obtido %f", norm)
	}
}

func TestEncodeDecodeVector(t *testing.T) {
	original := EmbedText("comida restaurante sushi")
	encoded := EncodeVector(original)
	decoded := DecodeVector(encoded)

	if len(original) != len(decoded) {
		t.Fatalf("tamanho diferente: original=%d, decoded=%d", len(original), len(decoded))
	}

	for i := range original {
		if math.Abs(float64(original[i]-decoded[i])) > 1e-6 {
			t.Errorf("discrepância no índice %d: original=%f, decoded=%f", i, original[i], decoded[i])
		}
	}
}

func TestCosineSimilarity(t *testing.T) {
	v1 := EmbedText("estação de trem em Tóquio")
	v2 := EmbedText("localidade onde fica a estação")
	v3 := EmbedText("almoço comer peixe restaurante")

	sim12 := CosineSimilarity(v1, v2)
	sim13 := CosineSimilarity(v1, v3)

	if sim12 <= sim13 {
		t.Errorf("esperado similaridade estação-localidade (%f) > estação-comida (%f)", sim12, sim13)
	}

	simIdentical := CosineSimilarity(v1, v1)
	if math.Abs(float64(simIdentical-1.0)) > 1e-4 {
		t.Errorf("similaridade com o próprio vetor deve ser ~1.0, obtido %f", simIdentical)
	}
}
