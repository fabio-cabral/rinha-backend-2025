package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3" // Driver do SQLite. O _ significa que é importado por seus efeitos colaterais (registrar o driver).
)

// DatabaseService encapsula a conexão com o banco de dados.
type DatabaseService struct {
	db *sql.DB
}

// NewDatabaseService cria o serviço de banco de dados, abre a conexão e inicializa o schema.
func NewDatabaseService(dbPath string) (*DatabaseService, error) {
	// Abre a conexão com o arquivo do banco de dados. O arquivo será criado se não existir.
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL") // WAL mode é melhor para concorrência
	if err != nil {
		return nil, fmt.Errorf("falha ao abrir o banco de dados: %w", err)
	}

	// Verifica se a conexão é válida.
	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("falha ao conectar com o banco de dados: %w", err)
	}

	// Lê o arquivo schema.sql e o executa para garantir que a tabela exista.
	schemaSQL, err := os.ReadFile("./sql/schema.sql")
	if err != nil {
		return nil, fmt.Errorf("falha ao ler o arquivo schema.sql: %w", err)
	}

	if _, err = db.Exec(string(schemaSQL)); err != nil {
		return nil, fmt.Errorf("falha ao executar o schema do banco de dados: %w", err)
	}

	log.Printf("Banco de dados inicializado com sucesso em: %s", dbPath)
	return &DatabaseService{db: db}, nil
}

// Close fecha a conexão com o banco de dados.
func (s *DatabaseService) Close() {
	s.db.Close()
}

// SavePayment salva o resultado de um pagamento processado no banco de dados.
func (s *DatabaseService) SavePayment(correlationID string, amount float64, processor string) {
	query := `INSERT INTO payments (correlation_id, amount, processor, processed_at) VALUES (?, ?, ?, ?)`
	
	// Não retornamos o erro aqui, apenas logamos, pois a falha ao salvar no BD não deve impedir o processamento.
	// Esta é uma decisão de design para a Rinha, para priorizar a velocidade de processamento.
	_, err := s.db.Exec(query, correlationID, amount, processor, time.Now().UTC())
	if err != nil {
		log.Printf("ERRO: Falha ao salvar pagamento %s no banco de dados: %v", correlationID, err)
	}
}

// GetSummary consulta o banco de dados e retorna um resumo agregado dos pagamentos.
func (s *DatabaseService) GetSummary() (*PaymentsSummaryResponse, error) {
	query := `SELECT processor, COUNT(*), SUM(amount) FROM payments GROUP BY processor`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("falha ao consultar o resumo de pagamentos: %w", err)
	}
	defer rows.Close()

	summary := &PaymentsSummaryResponse{}

	// Itera sobre os resultados da consulta. Haverá no máximo duas linhas (uma para 'default', uma para 'fallback').
	for rows.Next() {
		var processor string
		var totalRequests int
		var totalAmount float64

		if err := rows.Scan(&processor, &totalRequests, &totalAmount); err != nil {
			return nil, fmt.Errorf("falha ao ler a linha do resumo: %w", err)
		}

		if processor == "default" {
			summary.Default.TotalRequests = totalRequests
			summary.Default.TotalAmount = totalAmount
		} else if processor == "fallback" {
			summary.Fallback.TotalRequests = totalRequests
			summary.Fallback.TotalAmount = totalAmount
		}
	}

	// Verifica se houve algum erro durante a iteração.
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("erro durante a iteração das linhas do resumo: %w", err)
	}

	return summary, nil
}
