package main

import (
	"errors"
	"log"
	"sync"
	"time"
)

// State representa os possíveis estados de um Circuit Breaker.
type State int

const (
	StateClosed   State = iota // O circuito está fechado, operações são permitidas.
	StateOpen                // O circuito está aberto, operações são bloqueadas.
	StateHalfOpen            // O circuito está em teste, uma única operação é permitida.
)

// String para facilitar a visualização do estado em logs.
func (s State) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF-OPEN"
	}
	return "UNKNOWN"
}

// ErrCircuitIsOpen é retornado quando uma operação é tentada enquanto o circuito está aberto.
var ErrCircuitIsOpen = errors.New("circuit breaker está aberto")

// CircuitBreaker implementa a lógica do padrão.
type CircuitBreaker struct {
	mu                  sync.Mutex
	state               State
	failureThreshold    int
	resetTimeout        time.Duration
	consecutiveFailures int
	lastFailureTime     time.Time
}

// NewCircuitBreaker cria uma nova instância do CircuitBreaker.
// failureThreshold: número de falhas consecutivas para abrir o circuito.
// resetTimeout: tempo que o circuito permanece aberto antes de tentar se recuperar.
func NewCircuitBreaker(failureThreshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		failureThreshold: failureThreshold,
		resetTimeout:     resetTimeout,
		state:            StateClosed,
	}
}

// Execute envolve a execução de uma função com a proteção do Circuit Breaker.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	cb.mu.Lock()

	// Verifica se o circuito deve ser movido de Open para Half-Open.
	if cb.state == StateOpen && time.Since(cb.lastFailureTime) > cb.resetTimeout {
		cb.state = StateHalfOpen
		cb.consecutiveFailures = 0 // Permite uma nova tentativa
		log.Printf("[CircuitBreaker] Estado mudou para %s", cb.state)
	}

	// Se estiver aberto, bloqueia a execução.
	if cb.state == StateOpen {
		cb.mu.Unlock()
		return ErrCircuitIsOpen
	}

	cb.mu.Unlock()

	// Executa a função.
	err := fn()

	// Bloqueia novamente para atualizar o estado com base no resultado.
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		// A operação falhou.
		cb.consecutiveFailures++
		// Se o limite de falhas foi atingido ou se falhou no estado de teste (Half-Open), abre o circuito.
		if cb.state == StateHalfOpen || cb.consecutiveFailures >= cb.failureThreshold {
			if cb.state != StateOpen {
				log.Printf("[CircuitBreaker] Limite de falhas atingido. Abrindo o circuito.")
				cb.state = StateOpen
				cb.lastFailureTime = time.Now()
			}
		}
		return err
	}

	// A operação foi bem-sucedida. Reseta o estado.
	if cb.consecutiveFailures > 0 {
		log.Printf("[CircuitBreaker] Sucesso! Resetando o contador de falhas.")
		cb.consecutiveFailures = 0
	}
	if cb.state == StateHalfOpen {
		log.Printf("[CircuitBreaker] Sucesso em Half-Open. Fechando o circuito.")
		cb.state = StateClosed
	}

	return nil
}
