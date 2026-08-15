package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

type NodeRecord struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Type          string    `json:"type"` // local, agent
	Host          string    `json:"host"`
	AuthToken     string    `json:"-"`
	Status        string    `json:"status"` // online, offline, degraded
	Role          string    `json:"role"`   // leader, standby, learner, isolated
	Term          uint64    `json:"term"`
	Priority      int       `json:"priority"`
	AdvertisedIP  string    `json:"advertised_ip"`
	NetworkHealth bool      `json:"network_health"`
	ServiceAvail  bool      `json:"service_avail"`
	ActiveSpokes  int       `json:"active_spokes"`
	WSRttMs       float64   `json:"ws_rtt_ms"`
	ProbeMode     string    `json:"probe_mode"` // hybrid, agent_only, active_only
	LastSeen      time.Time `json:"last_seen"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type WitnessProbeRecord struct {
	ID           int64     `json:"id"`
	TargetNodeID string    `json:"target_node_id"`
	ProbeType    string    `json:"probe_type"` // l3_nbma, l4_port, overlay_gre
	TargetIP     string    `json:"target_ip"`
	RttMs        float64   `json:"rtt_ms"`
	LossRate     float64   `json:"loss_rate"`
	Success      bool      `json:"success"`
	Detail       string    `json:"detail"`
	RecordedAt   time.Time `json:"recorded_at"`
}

type WitnessArbitrationRecord struct {
	ID              int64     `json:"id"`
	Term            uint64    `json:"term"`
	PrimaryNodeID   string    `json:"primary_node_id"`
	BackupNodeID    string    `json:"backup_node_id"`
	InvolvedNodeIDs []string  `json:"involved_node_ids"`
	Decision        string    `json:"decision"` // approve_backup_elevation, retain_primary, alert_split_brain
	Reason          string    `json:"reason"`
	RecordedAt      time.Time `json:"recorded_at"`
}

type WitnessClusterRecord struct {
	ClusterID  string    `json:"cluster_id"`
	Epoch      string    `json:"epoch"`
	Mode       string    `json:"mode"`
	Holder     string    `json:"holder"`
	Term       uint64    `json:"term"`
	Sequence   uint64    `json:"sequence"`
	SafeUntil  time.Time `json:"safe_until"`
	Transition string    `json:"transition"`
	Reason     string    `json:"reason"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type SpokeMetaRecord struct {
	ProtocolAddress string    `json:"protocol_address"`
	Alias           string    `json:"alias"`
	SiteName        string    `json:"site_name"`
	Contact         string    `json:"contact"`
	Notes           string    `json:"notes"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type UserRecord struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"` // admin, readonly
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type AuditLogRecord struct {
	ID        int64     `json:"id"`
	NodeID    string    `json:"node_id"`
	Operator  string    `json:"operator"`
	Action    string    `json:"action"`
	Params    string    `json:"params"`
	Success   bool      `json:"success"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

type ConfigHistoryRecord struct {
	ID        int64     `json:"id"`
	NodeID    string    `json:"node_id"`
	Content   string    `json:"content"`
	Comment   string    `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
}

func InitDB(dbPath string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	sqlDB.SetMaxOpenConns(1) // SQLite single writer safety

	database := &DB{DB: sqlDB}
	if err := database.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	// Seed default admin user (admin / admin123) if no users exist
	count, err := database.CountUsers()
	if err == nil && count == 0 {
		hashBytes, hashErr := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		if hashErr == nil {
			err := database.CreateUser(&UserRecord{
				ID:           "u-admin-default",
				Username:     "admin",
				PasswordHash: string(hashBytes),
				Role:         "admin",
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			})
			if err != nil {
				log.Printf("[DB] Note: auto seed admin user: %v", err)
			} else {
				log.Printf("[DB] Initialized default admin user (admin / admin123)")
			}
		}
	}

	return database, nil
}

func (d *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'readonly',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		host TEXT DEFAULT '',
		auth_token TEXT DEFAULT '',
		status TEXT DEFAULT 'offline',
		role TEXT DEFAULT 'unknown',
		term INTEGER DEFAULT 0,
		advertised_ip TEXT DEFAULT '',
		last_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS witness_probes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		target_node_id TEXT NOT NULL,
		probe_type TEXT NOT NULL,
		target_ip TEXT NOT NULL,
		rtt_ms REAL DEFAULT 0,
		loss_rate REAL DEFAULT 0,
		success INTEGER DEFAULT 1,
		detail TEXT DEFAULT '',
		recorded_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_probes_time ON witness_probes(recorded_at);
	CREATE INDEX IF NOT EXISTS idx_probes_node_time ON witness_probes(target_node_id, recorded_at);

	CREATE TABLE IF NOT EXISTS witness_arbitrations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		term INTEGER DEFAULT 0,
		primary_node_id TEXT DEFAULT '',
		backup_node_id TEXT DEFAULT '',
		involved_node_ids TEXT DEFAULT '[]',
		decision TEXT NOT NULL,
		reason TEXT DEFAULT '',
		recorded_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS witness_clusters (
		cluster_id TEXT PRIMARY KEY,
		epoch TEXT NOT NULL DEFAULT '',
		mode TEXT NOT NULL DEFAULT 'legacy',
		holder TEXT NOT NULL DEFAULT '',
		term INTEGER NOT NULL DEFAULT 0,
		sequence INTEGER NOT NULL DEFAULT 0,
		safe_until DATETIME,
		transition TEXT NOT NULL DEFAULT '',
		reason TEXT NOT NULL DEFAULT '',
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS spoke_metadata (
		protocol_address TEXT PRIMARY KEY,
		alias TEXT DEFAULT '',
		site_name TEXT DEFAULT '',
		contact TEXT DEFAULT '',
		notes TEXT DEFAULT '',
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS audit_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		node_id TEXT DEFAULT '',
		operator TEXT DEFAULT 'admin',
		action TEXT NOT NULL,
		params TEXT DEFAULT '',
		success INTEGER DEFAULT 1,
		detail TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS config_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		node_id TEXT DEFAULT '',
		content TEXT NOT NULL,
		comment TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	INSERT OR IGNORE INTO nodes (id, name, type, host, status, role)
	VALUES ('witness', 'Witness 见证仲裁中心', 'witness', '127.0.0.1', 'online', 'witness');
	`

	_, err := d.Exec(schema)
	if err != nil {
		return err
	}

	// Dynamic column migrations for node telemetry & probe modes
	_, _ = d.Exec(`ALTER TABLE nodes ADD COLUMN priority INTEGER DEFAULT 0`)
	_, _ = d.Exec(`ALTER TABLE nodes ADD COLUMN network_health INTEGER DEFAULT 1`)
	_, _ = d.Exec(`ALTER TABLE nodes ADD COLUMN service_avail INTEGER DEFAULT 1`)
	_, _ = d.Exec(`ALTER TABLE nodes ADD COLUMN active_spokes INTEGER DEFAULT 0`)
	_, _ = d.Exec(`ALTER TABLE nodes ADD COLUMN ws_rtt_ms REAL DEFAULT 0`)
	_, _ = d.Exec(`ALTER TABLE nodes ADD COLUMN probe_mode TEXT DEFAULT 'hybrid'`)
	_, _ = d.Exec(`ALTER TABLE witness_arbitrations ADD COLUMN involved_node_ids TEXT DEFAULT '[]'`)

	return nil
}

func (d *DB) AddAuditLog(nodeID, operator, action, params string, success bool, detail string) {
	succInt := 0
	if success {
		succInt = 1
	}
	_, err := d.Exec(
		`INSERT INTO audit_logs (node_id, operator, action, params, success, detail, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		nodeID, operator, action, params, succInt, detail, time.Now(),
	)
	if err != nil {
		log.Printf("failed to write audit log: %v", err)
	}
}

func (d *DB) GetAuditLogs(limit, offset int) ([]AuditLogRecord, int, error) {
	var total int
	err := d.QueryRow("SELECT COUNT(*) FROM audit_logs").Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := d.Query(
		`SELECT id, node_id, operator, action, params, success, detail, created_at
		 FROM audit_logs ORDER BY id DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []AuditLogRecord
	for rows.Next() {
		var l AuditLogRecord
		var succInt int
		if err := rows.Scan(&l.ID, &l.NodeID, &l.Operator, &l.Action, &l.Params, &succInt, &l.Detail, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		l.Success = (succInt == 1)
		logs = append(logs, l)
	}

	return logs, total, nil
}

func (d *DB) SaveSpokeMetadata(m SpokeMetaRecord) error {
	_, err := d.Exec(
		`INSERT INTO spoke_metadata (protocol_address, alias, site_name, contact, notes, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(protocol_address) DO UPDATE SET
		   alias=excluded.alias,
		   site_name=excluded.site_name,
		   contact=excluded.contact,
		   notes=excluded.notes,
		   updated_at=excluded.updated_at`,
		m.ProtocolAddress, m.Alias, m.SiteName, m.Contact, m.Notes, time.Now(),
	)
	return err
}

func (d *DB) GetSpokeMetadata(protocolAddress string) (*SpokeMetaRecord, error) {
	var m SpokeMetaRecord
	err := d.QueryRow(
		`SELECT protocol_address, alias, site_name, contact, notes, updated_at
		 FROM spoke_metadata WHERE protocol_address = ?`,
		protocolAddress,
	).Scan(&m.ProtocolAddress, &m.Alias, &m.SiteName, &m.Contact, &m.Notes, &m.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (d *DB) ListSpokeMetadata() (map[string]SpokeMetaRecord, error) {
	rows, err := d.Query(`SELECT protocol_address, alias, site_name, contact, notes, updated_at FROM spoke_metadata`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]SpokeMetaRecord)
	for rows.Next() {
		var m SpokeMetaRecord
		if err := rows.Scan(&m.ProtocolAddress, &m.Alias, &m.SiteName, &m.Contact, &m.Notes, &m.UpdatedAt); err != nil {
			return nil, err
		}
		result[m.ProtocolAddress] = m
	}
	return result, nil
}

func (d *DB) SaveWitnessProbe(p WitnessProbeRecord) error {
	succInt := 0
	if p.Success {
		succInt = 1
	}
	recordedAt := p.RecordedAt
	if recordedAt.IsZero() {
		recordedAt = time.Now()
	}
	_, err := d.Exec(
		`INSERT INTO witness_probes (target_node_id, probe_type, target_ip, rtt_ms, loss_rate, success, detail, recorded_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.TargetNodeID, p.ProbeType, p.TargetIP, p.RttMs, p.LossRate, succInt, p.Detail, recordedAt,
	)
	return err
}

func (d *DB) GetProbes(targetNodeID, probeType string, hours, maxPoints int) ([]WitnessProbeRecord, error) {
	if hours <= 0 {
		hours = 24
	}
	if maxPoints <= 0 {
		maxPoints = 200
	}
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	bucketSeconds := hours * 3600
	if maxPoints > 1 {
		bucketSeconds = (hours*3600 + maxPoints - 2) / (maxPoints - 1)
	}

	query := `WITH grouped AS (
		SELECT MIN(id) AS sample_id, target_node_id, probe_type,
			COALESCE(AVG(CASE WHEN success = 1 AND rtt_ms > 0 THEN rtt_ms END), 0) AS avg_rtt,
			MAX(CASE WHEN success = 0 THEN 1.0 ELSE loss_rate END) AS max_loss,
			MIN(success) AS all_success
		FROM witness_probes
		WHERE recorded_at >= ?`
	args := []interface{}{cutoff}

	if targetNodeID != "" && targetNodeID != "all" {
		query += ` AND target_node_id = ?`
		args = append(args, targetNodeID)
	}
	if probeType != "" && probeType != "all" {
		query += ` AND probe_type = ?`
		args = append(args, probeType)
	}

	query += `
		GROUP BY target_node_id, probe_type,
			CAST((unixepoch(substr(recorded_at, 1, 19)) - unixepoch(substr(?, 1, 19))) / ? AS INTEGER)
	)
	SELECT grouped.sample_id, grouped.target_node_id, grouped.probe_type, samples.target_ip,
		grouped.avg_rtt, grouped.max_loss, grouped.all_success, '', samples.recorded_at
	FROM grouped
	JOIN witness_probes AS samples ON samples.id = grouped.sample_id
	ORDER BY samples.recorded_at ASC`
	args = append(args, cutoff, bucketSeconds)

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var probes []WitnessProbeRecord
	for rows.Next() {
		var p WitnessProbeRecord
		var succInt int
		if err := rows.Scan(&p.ID, &p.TargetNodeID, &p.ProbeType, &p.TargetIP, &p.RttMs, &p.LossRate, &succInt, &p.Detail, &p.RecordedAt); err != nil {
			return nil, err
		}
		p.Success = (succInt == 1)
		probes = append(probes, p)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return probes, nil
}

func (d *DB) GetRecentProbes(targetNodeID string, limit int) ([]WitnessProbeRecord, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := d.Query(
		`SELECT id, target_node_id, probe_type, target_ip, rtt_ms, loss_rate, success, detail, recorded_at
		 FROM (
			SELECT id, target_node_id, probe_type, target_ip, rtt_ms, loss_rate, success, detail, recorded_at,
				ROW_NUMBER() OVER (PARTITION BY probe_type ORDER BY id DESC) AS row_number
			FROM witness_probes
			WHERE target_node_id = ?
		 )
		 WHERE row_number <= ?
		 ORDER BY id DESC`,
		targetNodeID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []WitnessProbeRecord
	for rows.Next() {
		var p WitnessProbeRecord
		var succInt int
		if err := rows.Scan(&p.ID, &p.TargetNodeID, &p.ProbeType, &p.TargetIP, &p.RttMs, &p.LossRate, &succInt, &p.Detail, &p.RecordedAt); err != nil {
			return nil, err
		}
		p.Success = (succInt == 1)
		list = append(list, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

func (d *DB) CleanupOldProbes(retentionHours int) error {
	if retentionHours <= 0 {
		retentionHours = 24
	}
	_, err := d.Exec(`DELETE FROM witness_probes WHERE recorded_at < ?`, time.Now().Add(-time.Duration(retentionHours)*time.Hour))
	return err
}

func (d *DB) SaveArbitration(a WitnessArbitrationRecord) error {
	involvedNodeIDs, err := json.Marshal(a.InvolvedNodeIDs)
	if err != nil {
		return err
	}
	_, err = d.Exec(
		`INSERT INTO witness_arbitrations (term, primary_node_id, backup_node_id, involved_node_ids, decision, reason, recorded_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.Term, a.PrimaryNodeID, a.BackupNodeID, string(involvedNodeIDs), a.Decision, a.Reason, time.Now(),
	)
	return err
}

func (d *DB) GetArbitrations(limit int) ([]WitnessArbitrationRecord, error) {
	rows, err := d.Query(
		`SELECT id, term, primary_node_id, backup_node_id, involved_node_ids, decision, reason, recorded_at
		 FROM witness_arbitrations ORDER BY id DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []WitnessArbitrationRecord
	for rows.Next() {
		var a WitnessArbitrationRecord
		var involvedNodeIDs string
		if err := rows.Scan(&a.ID, &a.Term, &a.PrimaryNodeID, &a.BackupNodeID, &involvedNodeIDs, &a.Decision, &a.Reason, &a.RecordedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(involvedNodeIDs), &a.InvolvedNodeIDs); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, nil
}

func (d *DB) SaveWitnessCluster(record WitnessClusterRecord) error {
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = time.Now()
	}
	_, err := d.Exec(
		`INSERT INTO witness_clusters
		 (cluster_id, epoch, mode, holder, term, sequence, safe_until, transition, reason, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(cluster_id) DO UPDATE SET epoch=excluded.epoch, mode=excluded.mode,
		 holder=excluded.holder, term=excluded.term, sequence=excluded.sequence,
		 safe_until=excluded.safe_until, transition=excluded.transition,
		 reason=excluded.reason, updated_at=excluded.updated_at`,
		record.ClusterID, record.Epoch, record.Mode, record.Holder, record.Term,
		record.Sequence, record.SafeUntil, record.Transition, record.Reason,
		record.UpdatedAt,
	)
	return err
}

func (d *DB) GetWitnessClusters() ([]WitnessClusterRecord, error) {
	rows, err := d.Query(
		`SELECT cluster_id, epoch, mode, holder, term, sequence, safe_until,
		 transition, reason, updated_at FROM witness_clusters ORDER BY cluster_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []WitnessClusterRecord
	for rows.Next() {
		var record WitnessClusterRecord
		var safeUntil sql.NullTime
		if err := rows.Scan(&record.ClusterID, &record.Epoch, &record.Mode,
			&record.Holder, &record.Term, &record.Sequence, &safeUntil,
			&record.Transition, &record.Reason, &record.UpdatedAt); err != nil {
			return nil, err
		}
		if safeUntil.Valid {
			record.SafeUntil = safeUntil.Time
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// User CRUD operations

func (d *DB) CreateUser(u *UserRecord) error {
	now := time.Now()
	if u.CreatedAt.IsZero() {
		u.CreatedAt = now
	}
	if u.UpdatedAt.IsZero() {
		u.UpdatedAt = now
	}
	_, err := d.Exec(
		`INSERT INTO users (id, username, password_hash, role, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		u.ID, u.Username, u.PasswordHash, u.Role, u.CreatedAt, u.UpdatedAt,
	)
	return err
}

func (d *DB) GetUserByUsername(username string) (*UserRecord, error) {
	var u UserRecord
	err := d.QueryRow(
		`SELECT id, username, password_hash, role, created_at, updated_at
		 FROM users WHERE username = ?`,
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (d *DB) GetUserByID(id string) (*UserRecord, error) {
	var u UserRecord
	err := d.QueryRow(
		`SELECT id, username, password_hash, role, created_at, updated_at
		 FROM users WHERE id = ?`,
		id,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (d *DB) ListUsers() ([]UserRecord, error) {
	rows, err := d.Query(
		`SELECT id, username, password_hash, role, created_at, updated_at
		 FROM users ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []UserRecord
	for rows.Next() {
		var u UserRecord
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (d *DB) UpdateUser(id string, role string, passwordHash string) error {
	now := time.Now()
	if passwordHash != "" {
		_, err := d.Exec(
			`UPDATE users SET role = ?, password_hash = ?, updated_at = ? WHERE id = ?`,
			role, passwordHash, now, id,
		)
		return err
	}
	_, err := d.Exec(
		`UPDATE users SET role = ?, updated_at = ? WHERE id = ?`,
		role, now, id,
	)
	return err
}

func (d *DB) UpdateUserPassword(id string, passwordHash string) error {
	now := time.Now()
	_, err := d.Exec(
		`UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`,
		passwordHash, now, id,
	)
	return err
}

func (d *DB) DeleteUser(id string) error {
	_, err := d.Exec(`DELETE FROM users WHERE id = ?`, id)
	return err
}

func (d *DB) CountUsers() (int, error) {
	var count int
	err := d.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

func (d *DB) CountAdmins() (int, error) {
	var count int
	err := d.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&count)
	return count, err
}

func (d *DB) GetNode(id string) (*NodeRecord, error) {
	var n NodeRecord
	var netHealthInt, srvAvailInt int
	err := d.QueryRow(
		`SELECT id, name, type, host, status, role, term, priority, advertised_ip, network_health, service_avail, active_spokes, ws_rtt_ms, probe_mode, last_seen, created_at, updated_at
		 FROM nodes WHERE id = ? LIMIT 1`,
		id,
	).Scan(&n.ID, &n.Name, &n.Type, &n.Host, &n.Status, &n.Role, &n.Term, &n.Priority, &n.AdvertisedIP, &netHealthInt, &srvAvailInt, &n.ActiveSpokes, &n.WSRttMs, &n.ProbeMode, &n.LastSeen, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, err
	}
	n.NetworkHealth = (netHealthInt == 1)
	n.ServiceAvail = (srvAvailInt == 1)
	return &n, nil
}

func (d *DB) GetNodeByNameOrHost(query string) (*NodeRecord, error) {
	var n NodeRecord
	var netHealthInt, srvAvailInt int
	err := d.QueryRow(
		`SELECT id, name, type, host, status, role, term, priority, advertised_ip, network_health, service_avail, active_spokes, ws_rtt_ms, probe_mode, last_seen, created_at, updated_at
		 FROM nodes WHERE id = ? OR name = ? OR host = ? OR advertised_ip = ? LIMIT 1`,
		query, query, query, query,
	).Scan(&n.ID, &n.Name, &n.Type, &n.Host, &n.Status, &n.Role, &n.Term, &n.Priority, &n.AdvertisedIP, &netHealthInt, &srvAvailInt, &n.ActiveSpokes, &n.WSRttMs, &n.ProbeMode, &n.LastSeen, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, err
	}
	n.NetworkHealth = (netHealthInt == 1)
	n.ServiceAvail = (srvAvailInt == 1)
	return &n, nil
}
