package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// ExternalPaymentPayload é a estrutura enviada para os serviços de pagamento externos.
type ExternalPaymentPayload struct {
	CorrelationID string  `json:"correlationId"`
	Amount        float64 `json:"amount"`
	RequestedAt   string  `json:"requestedAt"`
}

// PaymentProcessor encapsula a lógica de processamento de pagamentos.
type PaymentProcessor struct {
	httpClient         *http.Client
	defaultURL         string
	fallbackURL        string
	defaultCB          *CircuitBreaker
	fallbackCB         *CircuitBreaker
	dbSvc              *DatabaseService
}

// NewPaymentProcessor cria uma nova instância do PaymentProcessor.
func NewPaymentProcessor(
	client *http.Client,
	defaultURL, fallbackURL string,
	defaultCB, fallbackCB *CircuitBreaker,
	dbSvc *DatabaseService,
) *PaymentProcessor {
	return &PaymentProcessor{
		httpClient:         client,
		defaultURL:         defaultURL,
		fallbackURL:        fallbackURL,
		defaultCB:          defaultCB,
		fallbackCB:         fallbackCB,
		dbSvc:              dbSvc,
	}
}

// StartWorker inicia um loop infinito para processar pagamentos de uma fila (canal).
func (p *PaymentProcessor) StartWorker(queue <-chan PaymentRequest) {
	// O loop 'for range' em um canal bloqueia até que um item esteja disponível.
	for payment := range queue {
		log.Printf("[Worker] Processando pagamento: %s", payment.CorrelationID)

		// 1. Tenta processar com o provedor 'default'.
		// A função a ser executada é encapsulada em uma closure.
		defaultWork := func() error {
			return p.sendPayment(payment, p.defaultURL)
		}

		err := p.defaultCB.Execute(defaultWork)
		if err == nil {
			// Sucesso! Salva no banco e vai para o próximo pagamento.
			log.Printf("[Worker] Pagamento %s processado com sucesso pelo 'default'", payment.CorrelationID)
			p.dbSvc.SavePayment(payment.CorrelationID, payment.Amount, "default")
			continue // Pula para a próxima iteração do loop
		}

		log.Printf("[Worker] Falha ao processar com 'default' para %s: %v. Tentando 'fallback'.", payment.CorrelationID, err)

		// 2. Se o 'default' falhou, tenta com o 'fallback'.
		fallbackWork := func() error {
			return p.sendPayment(payment, p.fallbackURL)
		}

		err = p.fallbackCB.Execute(fallbackWork)
		if err == nil {
			// Sucesso no fallback! Salva no banco.
			log.Printf("[Worker] Pagamento %s processado com sucesso pelo 'fallback'", payment.CorrelationID)
			p.dbSvc.SavePayment(payment.CorrelationID, payment.Amount, "fallback")
			continue // Pula para a próxima iteração do loop
		}

		// 3. Se ambos falharam, o pagamento é perdido.
		// Em um sistema real, aqui teríamos uma fila de 'dead letters' ou outra estratégia de retry.
		log.Printf("[Worker] ERRO FINAL: Falha ao processar pagamento %s com ambos os provedores. Erro final: %v", payment.CorrelationID, err)
	}
}

// sendPayment formata e envia a requisição de pagamento para a URL do processador.
func (p *PaymentProcessor) sendPayment(payment PaymentRequest, url string) error {
	payload := ExternalPaymentPayload{
		CorrelationID: payment.CorrelationID,
		Amount:        payment.Amount,
		RequestedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		// Este é um erro de programação, não de rede.
		return fmt.Errorf("falha ao serializar o payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url+"/payments", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("falha ao criar a requisição: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("falha na chamada HTTP: %w", err)
	}
	defer resp.Body.Close()

	// O desafio especifica que erros 5xx indicam instabilidade.
	// Qualquer resposta que não seja 2xx é considerada uma falha.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("resposta inesperada do servidor: %s", resp.Status)
	}

	return nil // Sucesso
}
