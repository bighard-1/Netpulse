package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

func (r *Repository) UpsertAdmin(username, passwordHash string) error {
	const q = `
		INSERT INTO users (username, password_hash, role)
		VALUES ($1, $2, 'admin')
		ON CONFLICT (username) DO UPDATE
		SET password_hash = EXCLUDED.password_hash,
		    role = 'admin';
	`
	if _, err := r.db.Exec(q, username, passwordHash); err != nil {
		return fmt.Errorf("upsert admin failed: %w", err)
	}
	return nil
}
func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	const q = `
		SELECT id, username, password_hash, role
		FROM users
		WHERE username = $1;
	`
	var u User
	if err := r.db.QueryRowContext(ctx, q, username).Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.Role,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get user failed: %w", err)
	}
	return &u, nil
}
func (r *Repository) CreateUser(ctx context.Context, username, passwordHash, role string) error {
	const q = `
		INSERT INTO users (username, password_hash, role)
		VALUES ($1, $2, $3);
	`
	if _, err := r.db.ExecContext(ctx, q, username, passwordHash, role); err != nil {
		return fmt.Errorf("create user failed: %w", err)
	}
	return nil
}
func (r *Repository) ListUsers(ctx context.Context) ([]User, error) {
	const q = `
		SELECT id, username, password_hash, role
		FROM users
		ORDER BY id;
	`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list users failed: %w", err)
	}
	defer rows.Close()
	out := make([]User, 0)
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role); err != nil {
			return nil, fmt.Errorf("scan users failed: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
func (r *Repository) HasPermission(ctx context.Context, userID int64, role, permission string) (bool, error) {
	if role == "admin" {
		return true, nil
	}
	if userID > 0 {
		const uq = `SELECT EXISTS(SELECT 1 FROM user_permissions WHERE user_id=$1 AND permission=$2);`
		var ok bool
		if err := r.db.QueryRowContext(ctx, uq, userID, permission).Scan(&ok); err == nil && ok {
			return true, nil
		}
	}
	const q = `SELECT EXISTS(SELECT 1 FROM role_permissions WHERE role=$1 AND (permission=$2 OR permission='*'));`
	var ok bool
	if err := r.db.QueryRowContext(ctx, q, role, permission).Scan(&ok); err != nil {
		return false, fmt.Errorf("check permission: %w", err)
	}
	return ok, nil
}
func (r *Repository) UpdateUser(ctx context.Context, id int64, username, role string, passwordHash *string) error {
	if passwordHash != nil && *passwordHash != "" {
		_, err := r.db.ExecContext(ctx, `UPDATE users SET username=$2, role=$3, password_hash=$4 WHERE id=$1;`, id, username, role, *passwordHash)
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE users SET username=$2, role=$3 WHERE id=$1;`, id, username, role)
	return err
}
func (r *Repository) DeleteUser(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id=$1;`, id)
	return err
}
func (r *Repository) ListUserPermissions(ctx context.Context, userID int64) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT permission FROM user_permissions WHERE user_id=$1 ORDER BY permission;`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (r *Repository) ReplaceUserPermissions(ctx context.Context, userID int64, permissions []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_permissions WHERE user_id=$1;`, userID); err != nil {
		return err
	}
	for _, p := range permissions {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO user_permissions(user_id,permission) VALUES($1,$2) ON CONFLICT (user_id,permission) DO NOTHING;`, userID, p); err != nil {
			return err
		}
	}
	return tx.Commit()
}
