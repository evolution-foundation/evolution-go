package poll_service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"github.com/evolution-foundation/evolution-go/pkg/poll/model"
	"github.com/google/uuid"
	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// PollService define a interface para gerenciamento de votos de enquetes
type PollService interface {
	// SavePollVote salva um voto de enquete no banco de dados
	SavePollVote(ctx context.Context, vote *model.PollVote) error

	// GetPollResults retorna os resultados de uma enquete
	GetPollResults(ctx context.Context, pollMessageID string, instanceID string) (*model.PollResults, error)
}

type pollService struct {
	db            *sql.DB
	loggerWrapper *logger_wrapper.LoggerManager
}

// NewPollService cria uma nova instância do serviço de polls
func NewPollService(db *sql.DB, loggerWrapper *logger_wrapper.LoggerManager) PollService {
	service := &pollService{
		db:            db,
		loggerWrapper: loggerWrapper,
	}

	// Auto-migration: criar tabela se não existir
	if err := service.autoMigrate(); err != nil {
		loggerWrapper.GetLogger("poll-service").LogError("[POLL] Auto-migration failed: %v", err)
	}

	return service
}

// autoMigrate cria a tabela poll_votes se não existir
func (s *pollService) autoMigrate() error {
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS poll_votes (
			id VARCHAR(255) PRIMARY KEY,
			company_id VARCHAR(255) NOT NULL,
			instance_id VARCHAR(255) NOT NULL,
			poll_message_id VARCHAR(255) NOT NULL,
			poll_chat_jid VARCHAR(255) NOT NULL,
			vote_message_id VARCHAR(255) NOT NULL,
			voter_jid VARCHAR(255) NOT NULL,
			voter_phone VARCHAR(255),
			voter_name VARCHAR(255),
			selected_options TEXT[] NOT NULL DEFAULT '{}',
			voted_at TIMESTAMP NOT NULL DEFAULT NOW(),
			received_at TIMESTAMP NOT NULL DEFAULT NOW(),
			CONSTRAINT unique_vote_per_poll UNIQUE (poll_message_id, voter_jid)
		);
		
		CREATE INDEX IF NOT EXISTS idx_poll_votes_company ON poll_votes(company_id);
		CREATE INDEX IF NOT EXISTS idx_poll_votes_instance ON poll_votes(instance_id);
		CREATE INDEX IF NOT EXISTS idx_poll_votes_poll_message ON poll_votes(poll_message_id);
		CREATE INDEX IF NOT EXISTS idx_poll_votes_chat ON poll_votes(poll_chat_jid);
		CREATE INDEX IF NOT EXISTS idx_poll_votes_voter ON poll_votes(voter_jid);
	`

	s.loggerWrapper.GetLogger("poll-service").LogInfo("[POLL] Running auto-migration...")

	_, err := s.db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create poll_votes table: %w", err)
	}

	return nil
}

// SavePollVote salva um voto de enquete no banco de dados (NÃO-INVASIVO)
func (s *pollService) SavePollVote(ctx context.Context, vote *model.PollVote) error {
	// Log seguro - não expõe dados sensíveis
	s.loggerWrapper.GetLogger("poll-service").LogInfo("[POLL] Saving vote for poll %s", vote.PollMessageID)

	// Extrair telefone do JID de forma segura
	if vote.VoterPhone == "" && strings.Contains(vote.VoterJid, "@") {
		phone := strings.Split(vote.VoterJid, "@")[0]
		vote.VoterPhone = phone
	}

	// Garantir ID único
	if vote.ID == "" {
		vote.ID = uuid.New().String()
	}

	// Query INSERT com ON CONFLICT para evitar duplicatas (SEGURO)
	query := `
		INSERT INTO poll_votes (
			id, company_id, instance_id, poll_message_id, poll_chat_jid,
			vote_message_id, voter_jid, voter_phone, voter_name,
			selected_options, voted_at, received_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10, $11, $12
		)
		ON CONFLICT (poll_message_id, voter_jid)
		DO UPDATE SET
			selected_options = EXCLUDED.selected_options,
			voted_at = EXCLUDED.voted_at,
			received_at = EXCLUDED.received_at
	`

	_, err := s.db.ExecContext(ctx, query,
		vote.ID,
		vote.CompanyID,
		vote.InstanceID,
		vote.PollMessageID,
		vote.PollChatJid,
		vote.VoteMessageID,
		vote.VoterJid,
		vote.VoterPhone,
		vote.VoterName,
		stringArrayToPostgresArray(vote.SelectedOptions),
		vote.VotedAt,
		vote.ReceivedAt,
	)

	if err != nil {
		s.loggerWrapper.GetLogger("poll-service").LogError("[POLL] Failed to save vote: %v", err)
		return fmt.Errorf("failed to save poll vote: %w", err)
	}

	s.loggerWrapper.GetLogger("poll-service").LogInfo("[POLL] Vote saved successfully for poll %s", vote.PollMessageID)
	return nil
}

// GetPollResults retorna os resultados agregados de uma enquete
func (s *pollService) GetPollResults(ctx context.Context, pollMessageID string, instanceID string) (*model.PollResults, error) {
	s.loggerWrapper.GetLogger("poll-service").LogInfo("[POLL] Fetching results for poll %s", pollMessageID)

	query := `
		SELECT 
			id, company_id, instance_id, poll_message_id, poll_chat_jid,
			vote_message_id, voter_jid, voter_phone, voter_name,
			selected_options, voted_at, received_at
		FROM poll_votes
		WHERE poll_message_id = $1 AND instance_id = $2
		ORDER BY voted_at ASC
	`

	rows, err := s.db.QueryContext(ctx, query, pollMessageID, instanceID)
	if err != nil {
		s.loggerWrapper.GetLogger("poll-service").LogError("[POLL] Failed to query votes: %v", err)
		return nil, fmt.Errorf("failed to query poll votes: %w", err)
	}
	defer rows.Close()

	var votes []model.PollVote
	optionCounts := make(map[string]int)

	for rows.Next() {
		var vote model.PollVote
		var selectedOptionsStr string

		err := rows.Scan(
			&vote.ID,
			&vote.CompanyID,
			&vote.InstanceID,
			&vote.PollMessageID,
			&vote.PollChatJid,
			&vote.VoteMessageID,
			&vote.VoterJid,
			&vote.VoterPhone,
			&vote.VoterName,
			&selectedOptionsStr,
			&vote.VotedAt,
			&vote.ReceivedAt,
		)
		if err != nil {
			s.loggerWrapper.GetLogger("poll-service").LogError("[POLL] Failed to scan vote: %v", err)
			continue
		}

		// Converter array do PostgreSQL para []string
		vote.SelectedOptions = postgresArrayToStringSlice(selectedOptionsStr)

		// Contar opções
		for _, option := range vote.SelectedOptions {
			optionCounts[option]++
		}

		votes = append(votes, vote)
	}

	if err = rows.Err(); err != nil {
		s.loggerWrapper.GetLogger("poll-service").LogError("[POLL] Rows iteration error: %v", err)
		return nil, fmt.Errorf("error iterating votes: %w", err)
	}

	// Verificar se há votos antes de construir resultado
	if len(votes) == 0 {
		s.loggerWrapper.GetLogger("poll-service").LogInfo("[POLL] No votes found for poll %s", pollMessageID)
		return &model.PollResults{
			PollMessageID: pollMessageID,
			PollChatJid:   "",
			TotalVotes:    0,
			Votes:         []model.PollVote{},
			OptionCounts:  make(map[string]int),
			Voters:        []model.VoterInfo{},
		}, nil
	}

	// Construir informações dos votantes
	voters := make([]model.VoterInfo, len(votes))
	for i, vote := range votes {
		voters[i] = model.VoterInfo{
			Jid:             vote.VoterJid,
			Phone:           vote.VoterPhone,
			Name:            vote.VoterName,
			SelectedOptions: vote.SelectedOptions,
			VotedAt:         vote.VotedAt,
		}
	}

	results := &model.PollResults{
		PollMessageID: pollMessageID,
		PollChatJid:   votes[0].PollChatJid,
		TotalVotes:    len(votes),
		Votes:         votes,
		OptionCounts:  optionCounts,
		Voters:        voters,
	}

	s.loggerWrapper.GetLogger("poll-service").LogInfo("[POLL] Found %d votes for poll %s", len(votes), pollMessageID)
	return results, nil
}

// Helper para converter []string em formato PostgreSQL array
func stringArrayToPostgresArray(arr []string) string {
	if len(arr) == 0 {
		return "{}"
	}
	return fmt.Sprintf("{%s}", strings.Join(arr, ","))
}

// Helper para converter array PostgreSQL de volta para []string
func postgresArrayToStringSlice(s string) []string {
	// Remove { e }
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")

	if s == "" {
		return []string{}
	}

	return strings.Split(s, ",")
}

// BuildPollVoteFromEvent constrói um model.PollVote a partir de eventos do WhatsApp (HELPER SEGURO)
// NOTA: Espera que voteInfo já tenha passado pelo JID swap (Sender = número real)
func BuildPollVoteFromEvent(
	pollInfo *types.MessageInfo,
	voteInfo *types.MessageInfo,
	decryptedVote *waProto.PollVoteMessage,
	companyID string,
	instanceID string,
) *model.PollVote {
	// Extrair opções selecionadas (hashes SHA-256)
	selectedOptions := make([]string, len(decryptedVote.SelectedOptions))
	for i, option := range decryptedVote.SelectedOptions {
		selectedOptions[i] = fmt.Sprintf("%x", option) // Converte bytes para hex
	}

	// Extrair telefone do votante
	// NOTA: O JID swap já foi feito antes de chegar aqui!
	// Se havia LID+WhatsApp, o Sender JÁ É o número real (@s.whatsapp.net) e SenderAlt é o LID
	voterPhone := voteInfo.Sender.User
	return &model.PollVote{
		ID:              uuid.New().String(),
		CompanyID:       companyID,
		InstanceID:      instanceID,
		PollMessageID:   pollInfo.ID,
		PollChatJid:     pollInfo.Chat.String(),
		VoteMessageID:   voteInfo.ID,
		VoterJid:        voteInfo.Sender.String(),
		VoterPhone:      voterPhone,
		VoterName:       voteInfo.PushName,
		SelectedOptions: selectedOptions,
		VotedAt:         voteInfo.Timestamp,
		ReceivedAt:      time.Now(),
	}
}
