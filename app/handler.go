package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

// handlePaymentRequest recebe, valida e enfileira um pagamento
func handlePaymentRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	var req PaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Corpo da requisição inválido", http.StatusBadRequest)
		return
	}

	if req.CorrelationID == "" || req.Amount <= 0 {
		http.Error(w, "Campos obrigatórios ausentes ou inválidos", http.StatusBadRequest)
		return
	}

	// Adiciona o pagamento na fila para ser processado de forma assíncrona
	paymentQueue <- req

	// Retorna 202 Accepted imediatamente
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("Pagamento recebido e enfileirado para processamento."))
}

// Summary representa o resumo de pagamentos
type Summary struct {
	TotalRequests int     `json:"totalRequests"`
	TotalAmount   float64 `json:"totalAmount"`
}

// PaymentsSummaryResponse é a estrutura da resposta final
type PaymentsSummaryResponse struct {
	Default  Summary `json:"default"`
	Fallback Summary `json:"fallback"`
}

// handlePaymentsSummary agrega os dados locais com os do outro container
func handlePaymentsSummary(w http.ResponseWriter, r *http.Request) {
	// Pega os dados locais
	localSummary, err := dbSvc.GetSummary()
	if err != nil {
		http.Error(w, "Erro ao buscar resumo local", http.StatusInternalServerError)
		return
	}

	// Pega os dados do outro container (peer)
	peerURL := os.Getenv("PEER_URL")
	peerSummary := &PaymentsSummaryResponse{}
	resp, err := http.Get(peerURL + "/internal-summary")
	if err == nil && resp.StatusCode == http.StatusOK {
		json.NewDecoder(resp.Body).Decode(peerSummary)
		resp.Body.Close()
	} else {
		log.Printf("Aviso: falha ao contatar o peer %s para o resumo. Err: %v", peerURL, err)
	}

	// Agrega os resultados
	finalSummary := PaymentsSummaryResponse{
		Default: Summary{
			TotalRequests: localSummary.Default.TotalRequests + peerSummary.Default.TotalRequests,
			TotalAmount:   localSummary.Default.TotalAmount + peerSummary.Default.TotalAmount,
		},
		Fallback: Summary{
			TotalRequests: localSummary.Fallback.TotalRequests + peerSummary.Fallback.TotalRequests,
			TotalAmount:   localSummary.Fallback.TotalAmount + peerSummary.Fallback.TotalAmount,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(finalSummary)
}

// handleInternalSummary é o endpoint que o outro container chama
func handleInternalSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := dbSvc.GetSummary()
	if err != nil {
		http.Error(w, "Erro ao buscar resumo interno", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(summary)
}
