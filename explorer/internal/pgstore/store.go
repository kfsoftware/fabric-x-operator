package pgstore

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"

	_ "github.com/lib/pq"
)

type Store struct {
	db *sql.DB
}

type TxRecord struct {
	TxID       string `json:"txId"`
	HeightHex  string `json:"heightHex"`
	BlockNum   uint64 `json:"blockNum"`
	TxNum      uint32 `json:"txNum"`
	Status     int32  `json:"status"`
	StatusName string `json:"statusName"`
}

type StateRecord struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version uint64 `json:"version"`
}

func Open(dsn string) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) GetHeight(ctx context.Context) (uint64, error) {
	var val []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT value FROM metadata WHERE key = '\x6c61737420636f6d6d697474656420626c6f636b206e756d626572'`).Scan(&val)
	if err != nil {
		return 0, fmt.Errorf("get height: %w", err)
	}
	return decodeHeight(val), nil
}

func (s *Store) GetTransactions(ctx context.Context, limit, offset int) ([]TxRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT tx_id, encode(height, 'hex'), status FROM tx_status ORDER BY height DESC LIMIT $1 OFFSET $2`,
		limit, offset)
	if err != nil {
		return nil, fmt.Errorf("get transactions: %w", err)
	}
	defer rows.Close()

	var records []TxRecord
	for rows.Next() {
		var r TxRecord
		var txIDBytes []byte
		if err := rows.Scan(&txIDBytes, &r.HeightHex, &r.Status); err != nil {
			return nil, fmt.Errorf("scan tx: %w", err)
		}
		r.TxID = decodeTxID(txIDBytes)
		r.BlockNum, r.TxNum = parseHeight(r.HeightHex)
		r.StatusName = statusName(r.Status)
		records = append(records, r)
	}
	return records, nil
}

func (s *Store) GetTransactionCount(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tx_status`).Scan(&count)
	return count, err
}

func (s *Store) GetTransactionByID(ctx context.Context, txID string) (*TxRecord, error) {
	var r TxRecord
	var txIDBytes []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT tx_id, encode(height, 'hex'), status FROM tx_status WHERE tx_id = $1::bytea`,
		[]byte(txID)).Scan(&txIDBytes, &r.HeightHex, &r.Status)
	if err != nil {
		return nil, fmt.Errorf("get tx %s: %w", txID, err)
	}
	r.TxID = decodeTxID(txIDBytes)
	r.BlockNum, r.TxNum = parseHeight(r.HeightHex)
	r.StatusName = statusName(r.Status)
	return &r, nil
}

func (s *Store) GetNamespaceState(ctx context.Context, namespace string, limit int) ([]StateRecord, error) {
	tableName := "ns_" + namespace
	query := fmt.Sprintf(`SELECT encode(key, 'hex'), COALESCE(encode(value, 'hex'), ''), version FROM %q ORDER BY key LIMIT $1`, tableName)
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}
	defer rows.Close()

	var records []StateRecord
	for rows.Next() {
		var r StateRecord
		if err := rows.Scan(&r.Key, &r.Value, &r.Version); err != nil {
			return nil, fmt.Errorf("scan state: %w", err)
		}
		records = append(records, r)
	}
	return records, nil
}

func (s *Store) GetTransactionsByBlock(ctx context.Context, blockNum uint64) ([]TxRecord, error) {
	// Height is encoded as big-endian: block_num (variable bytes) + tx_num (1 byte)
	// We need to find all txs where height starts with the block number prefix
	rows, err := s.db.QueryContext(ctx,
		`SELECT tx_id, encode(height, 'hex'), status FROM tx_status
		 WHERE height >= decode($1, 'hex') AND height < decode($2, 'hex')
		 ORDER BY height`,
		encodeBlockRange(blockNum),
		encodeBlockRange(blockNum+1))
	if err != nil {
		return nil, fmt.Errorf("get block txs: %w", err)
	}
	defer rows.Close()

	var records []TxRecord
	for rows.Next() {
		var r TxRecord
		var txIDBytes []byte
		if err := rows.Scan(&txIDBytes, &r.HeightHex, &r.Status); err != nil {
			return nil, fmt.Errorf("scan tx: %w", err)
		}
		r.TxID = decodeTxID(txIDBytes)
		r.BlockNum, r.TxNum = parseHeight(r.HeightHex)
		r.StatusName = statusName(r.Status)
		records = append(records, r)
	}
	return records, nil
}

func (s *Store) GetTransactionData(ctx context.Context, txID string) ([]StateRecord, error) {
	// Find all keys in token_namespace that contain this txid
	// Keys are: \000tr\000<txid>\000 (tx request) and \000<txid>\000<index>\000 (token outputs)
	txIDBytes := []byte(txID)
	rows, err := s.db.QueryContext(ctx,
		`SELECT encode(key, 'escape'), COALESCE(encode(value, 'hex'), ''), version
		 FROM ns_token_namespace
		 WHERE key LIKE $1
		 ORDER BY key`,
		append(append([]byte("%"), txIDBytes...), '%'))
	if err != nil {
		return nil, fmt.Errorf("get tx data: %w", err)
	}
	defer rows.Close()

	var records []StateRecord
	for rows.Next() {
		var r StateRecord
		if err := rows.Scan(&r.Key, &r.Value, &r.Version); err != nil {
			return nil, fmt.Errorf("scan tx data: %w", err)
		}
		records = append(records, r)
	}
	return records, nil
}

func (s *Store) GetAllNamespaceKeys(ctx context.Context, namespace string, txID string) ([]StateRecord, error) {
	tableName := "ns_" + namespace
	txIDBytes := []byte(txID)
	query := fmt.Sprintf(
		`SELECT encode(key, 'escape'), COALESCE(encode(value, 'hex'), ''), version FROM %q WHERE key LIKE $1 ORDER BY key`,
		tableName)
	rows, err := s.db.QueryContext(ctx, query, append(append([]byte("%"), txIDBytes...), '%'))
	if err != nil {
		return nil, fmt.Errorf("get ns keys: %w", err)
	}
	defer rows.Close()

	var records []StateRecord
	for rows.Next() {
		var r StateRecord
		if err := rows.Scan(&r.Key, &r.Value, &r.Version); err != nil {
			return nil, fmt.Errorf("scan ns key: %w", err)
		}
		records = append(records, r)
	}
	return records, nil
}

func (s *Store) ListNamespaces(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT tablename FROM pg_tables WHERE schemaname = 'public' AND tablename LIKE 'ns\_%' AND tablename NOT LIKE 'ns\_\_%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		// Strip ns_ prefix
		if len(name) > 3 {
			names = append(names, name[3:])
		}
	}
	return names, nil
}

func decodeHeight(val []byte) uint64 {
	var result uint64
	for _, b := range val {
		result = (result << 8) | uint64(b)
	}
	return result
}

func decodeTxID(val []byte) string {
	// tx_id is stored as the hex txid string in raw bytes
	return string(val)
}

func parseHeight(hexStr string) (uint64, uint32) {
	// Height encoding: [blockNumLen byte][blockNum bytes...][txNum byte]
	// Example: 012a00 → len=1, block=0x2a=42, tx=0
	// Example: 0000 → len=0, block=0, tx=0
	bytes, err := hex.DecodeString(hexStr)
	if err != nil || len(bytes) < 2 {
		return 0, 0
	}
	blockNumLen := int(bytes[0])
	if blockNumLen == 0 {
		// Special case: 0000 means block 0, tx 0
		return 0, uint32(bytes[1])
	}
	if len(bytes) < blockNumLen+2 {
		return 0, 0
	}
	blockNum := uint64(0)
	for i := 1; i <= blockNumLen; i++ {
		blockNum = (blockNum << 8) | uint64(bytes[i])
	}
	txNum := uint32(bytes[blockNumLen+1])
	return blockNum, txNum
}

func encodeBlockRange(blockNum uint64) string {
	// Height encoding: [blockNumLen][blockNum bytes][txNum=00]
	if blockNum == 0 {
		return "0000"
	}
	if blockNum < 0x100 {
		return fmt.Sprintf("01%02x00", blockNum)
	}
	if blockNum < 0x10000 {
		return fmt.Sprintf("02%04x00", blockNum)
	}
	return fmt.Sprintf("03%06x00", blockNum)
}

func statusName(status int32) string {
	switch status {
	case 0:
		return "UNSPECIFIED"
	case 1:
		return "COMMITTED"
	case 2:
		return "ABORTED_SIGNATURE_INVALID"
	case 3:
		return "ABORTED_MVCC_CONFLICT"
	default:
		return fmt.Sprintf("STATUS_%d", status)
	}
}
