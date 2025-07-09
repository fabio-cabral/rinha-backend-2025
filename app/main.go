package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

// PaymentRequest representa a estrutura de um pagamento recebido
type PaymentRequest struct {
	CorrelationID string  `json:"correlationId"`
	Amount        float64 `json:"amount"`
}

var (
	paymentQueue chan PaymentRequest
	dbSvc        *DatabaseService
)

func main() {
	// Configurações a partir de variáveis de ambiente
	dbPath := os.Getenv("DB_PATH")
	appPort := os.Getenv("APP_PORT")
	if appPort == "" {
		appPort = "8080"
	}

	// Inicializa o serviço de banco de dados
	var err error
	dbSvc, err = NewDatabaseService(dbPath)
	if err != nil {
		log.Fatalf("Falha ao iniciar o serviço de banco de dados: %v", err)
	}
	defer dbSvc.Close()

	// Inicializa a fila de pagamentos em memória (buffer de 10000)
	paymentQueue = make(chan PaymentRequest, 10000)

	// Inicializa os processadores com seus Circuit Breakers
	// Timeout de 10s para as chamadas HTTP
	client := &http.Client{Timeout: 10 * time.Second}
	defaultProcessorURL := os.Getenv("PAYMENT_PROCESSOR_URL_DEFAULT")
	fallbackProcessorURL := os.Getenv("PAYMENT_PROCESSOR_URL_FALLBACK")

	// 5 falhas consecutivas abrem o circuito por 30 segundos
	defaultCB := NewCircuitBreaker(5, 30*time.Second)
	fallbackCB := NewCircuitBreaker(5, 30*time.Second)

	processor := NewPaymentProcessor(client, defaultProcessorURL, fallbackProcessorURL, defaultCB, fallbackCB, dbSvc)

	// Inicia 100 workers para processar pagamentos da fila
	for i := 0; i < 100; i++ {
		go processor.StartWorker(paymentQueue)
	}

	// Configuração das rotas HTTP
	mux := http.NewServeMux()
	mux.HandleFunc("POST /payments", handlePaymentRequest)
	mux.HandleFunc("GET /payments-summary", handlePaymentsSummary)
	mux.HandleFunc("GET /internal-summary", handleInternalSummary) // Endpoint para agregação

	log.Printf("Servidor iniciado na porta %s", appPort)
	if err := http.ListenAndServe(":"+appPort, mux); err != nil {
		log.Fatalf("Erro ao iniciar servidor: %v", err)
	}
}
