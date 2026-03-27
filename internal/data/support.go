package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-kratos/kratos-layout/internal/biz"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
)

const supportSendIdempotentKey = "idemp:support:send:%s:%s"

type supportRepo struct {
	data *Data
	log  *log.Helper
}

// NewSupportRepo creates support repository.
func NewSupportRepo(data *Data, logger log.Logger) biz.SupportRepo {
	return &supportRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "data/support")),
	}
}

func (r *supportRepo) AcquireSendIdempotent(ctx context.Context, uid, clientReqID string, ttl time.Duration) (bool, error) {
	key := fmt.Sprintf(supportSendIdempotentKey, uid, clientReqID)
	return r.data.Redis().SetNX(ctx, key, "1", ttl).Result()
}

func (r *supportRepo) FindTurnByUIDAndClientReqID(ctx context.Context, uid, clientReqID string) (*biz.SessionTurn, error) {
	const query = `SELECT id, uid, session_id, client_req_id, user_message, assistant_reply, intent, used_fallback, model_name, prompt_tokens, reply_tokens, created_at
FROM support_turns
WHERE uid = ? AND client_req_id = ?
LIMIT 1`
	return r.scanOneTurn(ctx, query, uid, clientReqID)
}

func (r *supportRepo) EnsureSession(ctx context.Context, uid, sessionID string) (string, error) {
	if strings.TrimSpace(sessionID) == "" {
		sessionID = r.newSessionID()
		const stmt = `INSERT INTO support_sessions(uid, session_id, status) VALUES(?, ?, 1)`
		if _, err := r.data.DB().ExecContext(ctx, stmt, uid, sessionID); err != nil {
			return "", err
		}
		return sessionID, nil
	}
	ok, err := r.SessionExists(ctx, uid, sessionID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", biz.ErrSupportSessionNotFound
	}
	return sessionID, nil
}

func (r *supportRepo) SessionExists(ctx context.Context, uid, sessionID string) (bool, error) {
	const query = `SELECT 1 FROM support_sessions WHERE uid = ? AND session_id = ? AND deleted_at IS NULL LIMIT 1`
	var one int
	if err := r.data.DB().QueryRowContext(ctx, query, uid, sessionID).Scan(&one); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *supportRepo) ListRecentTurns(ctx context.Context, uid, sessionID string, limit int32) ([]*biz.SessionTurn, error) {
	const query = `SELECT id, uid, session_id, client_req_id, user_message, assistant_reply, intent, used_fallback, model_name, prompt_tokens, reply_tokens, created_at
FROM support_turns
WHERE uid = ? AND session_id = ?
ORDER BY id DESC
LIMIT ?`
	rows, err := r.data.DB().QueryContext(ctx, query, uid, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	desc := make([]*biz.SessionTurn, 0, limit)
	for rows.Next() {
		turn, scanErr := scanTurn(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		desc = append(desc, turn)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	// Reverse to chronological order for model context.
	asc := make([]*biz.SessionTurn, 0, len(desc))
	for i := len(desc) - 1; i >= 0; i-- {
		asc = append(asc, desc[i])
	}
	return asc, nil
}

func (r *supportRepo) CreateTurn(ctx context.Context, turn *biz.SessionTurn) error {
	const stmt = `INSERT INTO support_turns
(uid, session_id, client_req_id, user_message, assistant_reply, intent, used_fallback, model_name, prompt_tokens, reply_tokens)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := r.data.DB().ExecContext(ctx, stmt,
		turn.UID,
		turn.SessionID,
		turn.ClientReqID,
		turn.UserMessage,
		turn.AssistantReply,
		string(turn.Intent),
		boolToTinyInt(turn.UsedFallback),
		turn.ModelName,
		turn.PromptTokens,
		turn.ReplyTokens,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	turn.ID = uint64(id)
	turn.CreatedAt = time.Now()
	return nil
}

func (r *supportRepo) ListSessionTurns(ctx context.Context, uid, sessionID string, page, pageSize int32) ([]*biz.SessionTurn, error) {
	offset := (page - 1) * pageSize
	const query = `SELECT id, uid, session_id, client_req_id, user_message, assistant_reply, intent, used_fallback, model_name, prompt_tokens, reply_tokens, created_at
FROM support_turns
WHERE uid = ? AND session_id = ?
ORDER BY id DESC
LIMIT ? OFFSET ?`
	rows, err := r.data.DB().QueryContext(ctx, query, uid, sessionID, pageSize, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	turns := make([]*biz.SessionTurn, 0, pageSize)
	for rows.Next() {
		turn, scanErr := scanTurn(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		turns = append(turns, turn)
	}
	return turns, rows.Err()
}

func (r *supportRepo) scanOneTurn(ctx context.Context, query string, args ...any) (*biz.SessionTurn, error) {
	row := r.data.DB().QueryRowContext(ctx, query, args...)
	turn := &biz.SessionTurn{}
	var usedFallback int32
	if err := row.Scan(
		&turn.ID,
		&turn.UID,
		&turn.SessionID,
		&turn.ClientReqID,
		&turn.UserMessage,
		&turn.AssistantReply,
		&turn.Intent,
		&usedFallback,
		&turn.ModelName,
		&turn.PromptTokens,
		&turn.ReplyTokens,
		&turn.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	turn.UsedFallback = usedFallback == 1
	return turn, nil
}

func (r *supportRepo) newSessionID() string {
	return "cs_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func boolToTinyInt(v bool) int32 {
	if v {
		return 1
	}
	return 0
}

func scanTurn(rows *sql.Rows) (*biz.SessionTurn, error) {
	turn := &biz.SessionTurn{}
	var usedFallback int32
	if err := rows.Scan(
		&turn.ID,
		&turn.UID,
		&turn.SessionID,
		&turn.ClientReqID,
		&turn.UserMessage,
		&turn.AssistantReply,
		&turn.Intent,
		&usedFallback,
		&turn.ModelName,
		&turn.PromptTokens,
		&turn.ReplyTokens,
		&turn.CreatedAt,
	); err != nil {
		return nil, err
	}
	turn.UsedFallback = usedFallback == 1
	return turn, nil
}
