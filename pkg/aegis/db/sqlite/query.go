package sqlite

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/bctnry/aegis/pkg/aegis/db"
	"github.com/bctnry/aegis/pkg/aegis/model"
	"github.com/bctnry/aegis/pkg/gitlib"
	_ "github.com/mattn/go-sqlite3"
)

func (dbif *SqliteAegisDatabaseInterface) Dispose() error {
	return dbif.connection.Close()
}

func (dbif *SqliteAegisDatabaseInterface) GetUserByName(name string) (*model.AegisUser, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT user_name, user_title, user_email, user_bio, user_website, user_status, user_password_hash
FROM %suser
WHERE user_name = ?
`, pfx))
	if err != nil { return nil, err }
	var username, title, email, bio, website, ph string
	var status int
	err = stmt.QueryRow(name).Scan(&username, &title, &email, &bio, &website, &status, &ph)
	if err == sql.ErrNoRows { return nil, db.ErrEntityNotFound }
	if err != nil { return nil, err }
	return &model.AegisUser{
		Name: username,
		Title: title,
		Email: email,
		Bio: bio,
		Website: website,
		Status: model.AegisUserStatus(status),
		PasswordHash: ph,
	}, nil
}

func (dbif *SqliteAegisDatabaseInterface) GetAuthKeyByName(userName string, keyName string) (*model.AegisAuthKey, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT key_text
FROM %suser_authkey
WHERE user_name = ? AND key_name = ?
`, pfx))
	if err != nil { return nil, err }
	r := stmt.QueryRow(userName, keyName)
	err = r.Err()
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, db.ErrEntityNotFound
		}
		return nil, err
	}
	kt := ""
	err = r.Scan(&kt)
	if err != nil { return nil, err }
	return &model.AegisAuthKey{
		UserName: userName,
		KeyName: keyName,
		KeyText: kt,
	}, nil
}

func (dbif *SqliteAegisDatabaseInterface) GetAllAuthKeyByUsername(name string) ([]model.AegisAuthKey, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT key_name, key_text
FROM %suser_authkey
WHERE user_name = ?
`, pfx))
	if err != nil { return nil, err }
	r, err := stmt.Query(name)
	if err != nil { return nil, err }
	defer r.Close()
	res := make([]model.AegisAuthKey, 0)
	for r.Next() {
		var keyName, keyText string
		err = r.Scan(&keyName, &keyText)
		if err != nil { return nil, err }
		res = append(res, model.AegisAuthKey{
			UserName: name,
			KeyText: keyText,
			KeyName: keyName,
		})
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) RegisterAuthKey(username string, keyname string, keytext string) error {
	pfx := dbif.config.Database.TablePrefix
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT 1 FROM %suser_authkey WHERE user_name = ? AND key_name = ?
`, pfx))
	if err != nil { return err }
	r := stmt1.QueryRow(username, keyname)
	if r.Err() != nil { return r.Err() }
	var verdict string
	err = r.Scan(&verdict)
	if err != nil && err != sql.ErrNoRows { return err }
	if err == nil {
		return db.ErrEntityAlreadyExists
	}
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	stmt2, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %suser_authkey(user_name, key_name, key_text)
VALUES (?,?,?)
`, pfx))
	if err != nil { return err }
	_, err = stmt2.Exec(username, keyname, keytext)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) UpdateAuthKey(username string, keyname string, keytext string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
UPDATE %suser_authkey SET key_text = ? WHERE user_name = ? AND key_name = ?
`, pfx))
	if err != nil { return err }
	defer stmt.Close()
	_, err = stmt.Exec(keytext, username, keyname)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) RemoveAuthKey(username string, keyname string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	stmt, err := tx.Prepare(fmt.Sprintf(`
DELETE FROM %suser_authkey
WHERE user_name = ? AND key_name = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt.Exec(username, keyname)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) GetAllSignKeyByUsername(name string) ([]model.AegisSigningKey, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT key_name, key_text
FROM %suser_signkey
WHERE user_name = ?
`, pfx))
	if err != nil { return nil, err }
	r, err := stmt.Query(name)
	if err != nil { return nil, err }
	defer r.Close()
	res := make([]model.AegisSigningKey, 0)
	for r.Next() {
		var keyName, keyText string
		err = r.Scan(&keyName, &keyText)
		if err != nil { return nil, err }
		res = append(res, model.AegisSigningKey{
			UserName: name,
			KeyText: keyText,
			KeyName: keyName,
		})
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) GetSignKeyByName(userName string, keyName string) (*model.AegisSigningKey, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT key_text FROM %suser_signkey
WHERE user_name = ? AND key_name = ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt.Close()
	r := stmt.QueryRow(userName, keyName)
	if r.Err() != nil { return nil, r.Err() }
	var text string
	err = r.Scan(&text)
	if err != nil { return nil, err }
	return &model.AegisSigningKey{
		UserName: userName,
		KeyName: keyName,
		KeyText: text,
	}, nil
}

func (dbif *SqliteAegisDatabaseInterface) UpdateSignKey(username string, keyname string, keytext string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
UPDATE %suser_signkey SET key_text = ? WHERE user_name = ? AND key_name = ?
`, pfx))
	if err != nil { return err }
	defer stmt.Close()
	_, err = stmt.Exec(keytext, username, keyname)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) RegisterSignKey(username string, keyname string, keytext string) error {
	pfx := dbif.config.Database.TablePrefix
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT 1 FROM %suser_signkey WHERE user_name = ? AND key_name = ?
`, pfx))
	if err != nil { return err }
	r := stmt1.QueryRow(username, keyname)
	if r.Err() != nil { return r.Err() }
	var verdict string
	err = r.Scan(&verdict)
	if err != nil && err != sql.ErrNoRows { return err }
	if err == nil {
		return db.ErrEntityAlreadyExists
	}
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	stmt2, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %suser_signkey(user_name, key_name, key_text)
VALUES (?,?,?)
`, pfx))
	if err != nil { return err }
	_, err = stmt2.Exec(username, keyname, keytext)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) RemoveSignKey(username string, keyname string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	stmt, err := tx.Prepare(fmt.Sprintf(`
DELETE FROM %suser_signkey
WHERE user_name = ? AND key_name = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt.Exec(username, keyname)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) RegisterUser(name string, email string, passwordHash string, status model.AegisUserStatus) (*model.AegisUser, error) {
	pfx := dbif.config.Database.TablePrefix
	t := time.Now().Unix()
	tx, err := dbif.connection.Begin()
	if err != nil { return nil, err }
	stmt, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %suser(user_name, user_title, user_email, user_bio, user_website, user_password_hash, user_reg_datetime, user_status)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, pfx))
	if err != nil { return nil, err }
	_, err = stmt.Exec(name, name, email, "", "", passwordHash, t, status)
	if err != nil { tx.Rollback(); return nil, err }
	err = tx.Commit()
	if err != nil { tx.Rollback(); return nil, err }
	// we delete whatever we have inside the old ns.
	// this should be ok since when a user exists as "deleted"
	// state the actions above should've violated unique constraint
	// and triggered an error already. of course we should remove
	// "deleted" user regularly from the dbto prevent possible sabotage.
	userNsPath := path.Join(dbif.config.GitRoot, name)
	err = os.RemoveAll(userNsPath)
	if err != nil && !os.IsNotExist(err) { tx.Rollback(); return nil, err }
	err = os.MkdirAll(userNsPath, os.ModeDir|0755)
	// TODO: chown.
	if err != nil { tx.Rollback(); return nil, err }
	return &model.AegisUser{
		Name: name,
		Title: name,
		Email: email,
		PasswordHash: passwordHash,
		Bio: "",
		Website: "",
		RegisterTime: t,
	}, nil
}

func (dbif *SqliteAegisDatabaseInterface) UpdateUserInfo(name string, uobj *model.AegisUser) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	stmt, err := tx.Prepare(fmt.Sprintf(`
UPDATE %suser
SET
    user_title = ?, user_email = ?, user_bio = ?,
    user_website = ?, user_status = ?
WHERE
    user_name = ?
`, pfx))
	if err != nil { tx.Rollback(); return err }
	_, err = stmt.Exec(uobj.Title, uobj.Email, uobj.Bio, uobj.Website, uobj.Status, name)
	if err != nil { tx.Rollback(); return err }
	err = tx.Commit()
	if err != nil { tx.Rollback(); return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) UpdateUserPassword(name string, newPasswordHash string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	stmt, err := tx.Prepare(fmt.Sprintf(`
UPDATE %suser
SET user_password_hash = ?
WHERE user_name = ?
`, pfx))
	if err != nil { tx.Rollback(); return err }
	_, err = stmt.Exec(newPasswordHash, name)
	if err != nil { tx.Rollback(); return err }
	err = tx.Commit()
	if err != nil { tx.Rollback(); return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) UpdateUserStatus(name string, newStatus model.AegisUserStatus) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	stmt, err := tx.Prepare(fmt.Sprintf(`
UPDATE %suser
SET user_status = ?
WHERE user_name = ?
`, pfx))
	if err != nil { tx.Rollback(); return err }
	_, err = stmt.Exec(newStatus, name)
	if err != nil { tx.Rollback(); return err }
	err = tx.Commit()
	if err != nil { tx.Rollback(); return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) HardDeleteUserByName(name string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	stmt, err := tx.Prepare(fmt.Sprintf(`
DELETE FROM %suser WHERE user_name = ?
`, pfx))
	if err != nil { tx.Rollback(); return err }
	_, err = stmt.Exec(name)
	if err != nil { tx.Rollback(); return err }
	userNsPath := path.Join(dbif.config.GitRoot, name)
	err = os.RemoveAll(userNsPath)
	if err != nil { tx.Rollback(); return err }
	err = tx.Commit()
	if err != nil { tx.Rollback(); return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) GetNamespaceByName(name string) (*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %snamespace
WHERE ns_name = ?
`, pfx))
	if err != nil { return nil, err }
	r := stmt.QueryRow(name)
	if r.Err() != nil { return nil, r.Err() }
	var title, desc, email, owner, acl string
	var reg_date int64
	var status int
	err = r.Scan(&title, &desc, &email, &owner, &reg_date, &status, &acl)
	if err == sql.ErrNoRows {
		return nil, db.ErrEntityNotFound
	}
	if err != nil { return nil, err }
	a, err := model.ParseACL(acl)
	if err != nil { return nil, err }
	return &model.Namespace{
		Name: name,
		Title: title,
		Description: desc,
		Email: email,
		Owner: owner,
		RegisterTime: reg_date,
		ACL: a,
		Status: model.AegisNamespaceStatus(status),
	}, nil
}

func (dbif *SqliteAegisDatabaseInterface) GetRepositoryByName(nsName string, repoName string) (*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name
FROM %srepository
WHERE repo_namespace = ? AND repo_name = ?
`, pfx))
	if err != nil { return nil, err }
	r := stmt.QueryRow(nsName, repoName)
	if r.Err() != nil { return nil, r.Err() }
	var desc, owner, acl, forkOriginNs, forkOriginName string
	var status int
	var repoType uint8
	err = r.Scan(&repoType, &desc, &owner, &acl, &status, &forkOriginNs, &forkOriginName)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, db.ErrEntityNotFound
		}
		return nil, err
	}
	p := path.Join(dbif.config.GitRoot, nsName, repoName)
	res, err := model.NewRepository(nsName, repoName, gitlib.NewLocalGitRepository(nsName, repoName, p))
	res.Type = repoType
	res.Owner = owner
	res.Status = model.AegisRepositoryStatus(status)
	res.ForkOriginNamespace = forkOriginNs
	res.ForkOriginName = forkOriginName
	aclobj, err := model.ParseACL(acl)
	if err != nil { return nil, err }
	res.AccessControlList = aclobj
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) RegisterNamespace(name string, ownerUsername string) (*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return nil, err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %snamespace(ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl)
VALUES (?,?,?,?,?,?,?,?)
`, pfx))
	t := time.Now().Unix()
	if err != nil { return nil, err }
	_, err = stmt.Exec(name, name, "", "", ownerUsername, t, model.NAMESPACE_NORMAL_PUBLIC, "")
	if err != nil {
		// NOTE: this is here since the error value cannot be tested with
		// errors.Is w/ any error value defined in sqlite3 - you can but
		// the result wouldn't be right.
		// that's golang for you, a language without a proper, sane way
		// of dealing with errors.
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, db.ErrEntityAlreadyExists
		}
		return nil, err
	}
	nsPath := path.Join(dbif.config.GitRoot, name)
	err = os.RemoveAll(nsPath)
	if err != nil { return nil, err }
	err = os.Mkdir(nsPath, os.ModeDir|0755)
	if err != nil { return nil, err }
	err = tx.Commit()
	if err != nil { return nil, err }
	return &model.Namespace{
		Name: name,
		Title: name,
		Description: "",
		Email: "",
		Owner: ownerUsername,
		RegisterTime: t,
		Status: model.NAMESPACE_NORMAL_PUBLIC,
		ACL: nil,
		RepositoryList: nil,
		LocalPath: nsPath,
	}, nil
}

func (dbif *SqliteAegisDatabaseInterface) GetAllVisibleNamespace(username string) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	privateSelectClause := ""
	if len(username) > 0 {
		privateSelectClause = "OR (ns_owner = ? OR ns_acl LIKE ? ESCAPE ?)"
	}
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %snamespace
WHERE ns_status = 1 %s
`, pfx, privateSelectClause))
	if err != nil { return nil, err }
	defer stmt1.Close()
	var rs *sql.Rows
	if len(username) > 0 {
		pattern := ToSqlSearchPattern(username)
		rs, err = stmt1.Query(username, pattern, "\\")
	} else {
		rs, err = stmt1.Query()
	}
	if err != nil { return nil, err }
	defer rs.Close()
	res := make(map[string]*model.Namespace, 0)
	for rs.Next() {
		var name, title, desc, email, owner, acl string
		var regtime int64
		var status int64
		err = rs.Scan(&name, &title, &desc, &email, &owner, &regtime, &status, &acl)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		res[name] = &model.Namespace{
			Name: name,
			Title: title,
			Description: desc,
			Email: email,
			Owner: owner,
			RegisterTime: regtime,
			ACL: a,
			Status: model.AegisNamespaceStatus(status),
		}
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) GetAllNamespace() (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %snamespace
WHERE ns_status != 3 AND ns_status != 4
`, pfx))
	if err != nil { return nil, err }
	rs, err := stmt.Query()
	if err != nil { return nil, err }
	defer rs.Close()
	res := make(map[string]*model.Namespace, 0)
	for rs.Next() {
		var name, title, desc, email, owner, acl string
		var regtime int64
		var status int64
		err = rs.Scan(&name, &title, &desc, &email, &owner, &regtime, &status, &acl)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		res[name] = &model.Namespace{
			Name: name,
			Title: title,
			Description: desc,
			Email: email,
			Owner: owner,
			RegisterTime: regtime,
			ACL: a,
			Status: model.AegisNamespaceStatus(status),
		}
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) GetAllNamespaceByOwner(name string) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %snamespace
WHERE ns_status != 3 AND ns_status != 4 AND ns_owner = ?
`, pfx))
	if err != nil { return nil, err }
	rs, err := stmt.Query(name)
	if err != nil { return nil, err }
	defer rs.Close()
	res := make(map[string]*model.Namespace, 0)
	for rs.Next() {
		var name, title, desc, email, owner, acl string
		var regtime int64
		var status int64
		err = rs.Scan(&name, &title, &desc, &email, &owner, &regtime, &status, &acl)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		res[name] = &model.Namespace{
			Name: name,
			Title: title,
			Description: desc,
			Email: email,
			Owner: owner,
			RegisterTime: regtime,
			ACL: a,
			Status: model.AegisNamespaceStatus(status),
		}
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) GetAllVisibleRepositoryFromNamespace(username string, ns string) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	privateSelectClause := ""
	if len(username) > 0 {
		privateSelectClause = "OR (repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)"
	}
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name
FROM %srepository
WHERE repo_namespace = ? AND (repo_status = 1 OR repo_status = 4 %s)
`, pfx, privateSelectClause))
	if err != nil { return nil, err }
	var rs *sql.Rows
	if len(username) > 0 {
		pattern := strings.ReplaceAll(username, "\\", "\\\\")
		pattern = strings.ReplaceAll(pattern, "%", "\\%")
		pattern = strings.ReplaceAll(pattern, "_", "\\_")
		pattern = "%" + pattern + "%"
		rs, err = stmt.Query(ns, username, pattern, "\\")
	} else {
		rs, err = stmt.Query(ns)
	}
	if err != nil { return nil, err }
	defer rs.Close()
	res := make([]*model.Repository, 0)
	for rs.Next() {
		var name, desc, owner, acl, forkOriginName, forkOriginNs string
		var status int64
		err = rs.Scan(&name, &desc, &owner, &acl, &status, &forkOriginNs, &forkOriginName)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		p := path.Join(dbif.config.GitRoot, ns, name)
		res = append(res, &model.Repository{
			Namespace: ns,
			Name: name,
			Owner: owner,
			Description: desc,
			AccessControlList: a,
			Status: model.AegisRepositoryStatus(status),
			Repository: gitlib.NewLocalGitRepository(ns, name, p),
			ForkOriginNamespace: forkOriginNs,
			ForkOriginName: forkOriginName,
		})
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) GetAllRepositoryFromNamespace(ns string) (map[string]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name
FROM %srepository
WHERE repo_namespace = ?
`, pfx))
	if err != nil { return nil, err }
	rs, err := stmt.Query(ns)
	if err != nil { return nil, err }
	defer rs.Close()
	res := make(map[string]*model.Repository, 0)
	for rs.Next() {
		var name, desc, acl, owner, forkOriginNs, forkOriginName string
		var status int64
		err = rs.Scan(&name, &desc, &owner, &acl, &status)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		p := path.Join(dbif.config.GitRoot, ns, name)
		res[name] = &model.Repository{
			Namespace: ns,
			Name: name,
			Owner: owner,
			Description: desc,
			AccessControlList: a,
			Status: model.AegisRepositoryStatus(status),
			Repository: gitlib.NewLocalGitRepository(ns, name, p),
			ForkOriginNamespace: forkOriginNs,
			ForkOriginName: forkOriginName,
		}
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) UpdateNamespaceInfo(name string, nsobj *model.Namespace) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	stmt, err := tx.Prepare(fmt.Sprintf(`
UPDATE %snamespace
SET ns_title = ?, ns_description = ?, ns_email = ?, ns_owner = ?, ns_status = ?
WHERE ns_name = ?
`, pfx))
	if err != nil { tx.Rollback(); return err }
	_, err = stmt.Exec(nsobj.Title, nsobj.Description, nsobj.Email, nsobj.Owner, nsobj.Status, name)
	if err != nil { tx.Rollback(); return err }
	err = tx.Commit()
	if err != nil { tx.Rollback(); return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) UpdateNamespaceOwner(name string, newOwner string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	stmt, err := tx.Prepare(fmt.Sprintf(`
UPDATE %snamespace
SET ns_owner = ?
WHERE ns_name = ?
`, pfx))
	if err != nil { tx.Rollback(); return err }
	_, err = stmt.Exec(newOwner, name)
	if err != nil { tx.Rollback(); return err }
	err = tx.Commit()
	if err != nil { tx.Rollback(); return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) UpdateNamespaceStatus(name string, newStatus model.AegisNamespaceStatus) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	stmt, err := tx.Prepare(fmt.Sprintf(`
UPDATE %snamespace
SET ns_status = ?
WHERE ns_name = ?
`, pfx))
	if err != nil { tx.Rollback(); return err }
	_, err = stmt.Exec(newStatus, name)
	if err != nil { tx.Rollback(); return err }
	err = tx.Commit()
	if err != nil { tx.Rollback(); return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) HardDeleteNamespaceByName(name string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	stmt, err := tx.Prepare(fmt.Sprintf(`
DELETE FROM %snamespace WHERE ns_name = ?
`, pfx))
	if err != nil { tx.Rollback(); return err }
	_, err = stmt.Exec(name)
	if err != nil { tx.Rollback(); return err }
	nsPath := path.Join(dbif.config.GitRoot, name)
	err = os.RemoveAll(nsPath)
	if err != nil { tx.Rollback(); return err }
	err = tx.Commit()
	if err != nil { tx.Rollback(); return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) CreateRepository(ns string, name string, repoType uint8, owner string) (*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	fullName := ns + ":" + name
	tx, err := dbif.connection.Begin()
	if err != nil { return nil, err }
	defer tx.Rollback()
	stmt1, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %srepository(repo_type, repo_fullname, repo_namespace, repo_name, repo_description, repo_acl, repo_status, repo_owner, repo_fork_origin_namespace, repo_fork_origin_name)
VALUES (?,?,?,?,?,?,?,?,?)
`, pfx))
	if err != nil { return nil, err }
	_, err = stmt1.Exec(repoType, fullName, ns, name, new(string), new(string), model.REPO_NORMAL_PUBLIC, owner, new(string), new(string))
	if err != nil { return nil, err }
	p := path.Join(dbif.config.GitRoot, ns, name)
	err = os.RemoveAll(p)
	if err != nil { return nil, err }
	if err = os.MkdirAll(p, os.ModeDir|0775); err != nil {
		return nil, err
	}
	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = p
	if err = cmd.Run(); err != nil { return nil, err }
	if err = tx.Commit(); err != nil { return nil, err }
	r, err := model.NewRepository(ns, name, gitlib.NewLocalGitRepository(ns, name, p))
	r.Type = repoType
	r.Owner = owner
	if err != nil { return nil, err }
	return r, nil
}

func (dbif *SqliteAegisDatabaseInterface) SetUpCloneRepository(originNs string, originName string, targetNs string, targetName string, owner string) (*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	targetFullName := targetNs + ":" + targetName
	tx, err := dbif.connection.Begin()
	if err != nil { return nil, err }
	defer tx.Rollback()
	stmt1, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %srepository(repo_fullname, repo_namespace, repo_name, repo_description, repo_acl, repo_status, repo_owner, repo_fork_origin_namespace, repo_fork_origin_name)
VALUES (?,?,?,?,?,?,?,?,?)
`, pfx))
	if err != nil { return nil, err }
	_, err = stmt1.Exec(targetFullName, targetNs, targetName, new(string), new(string), model.REPO_NORMAL_PUBLIC, owner, originNs, originName)
	if err != nil {
		// TODO: find a better way to do this...
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, db.ErrEntityAlreadyExists
		} else {
			return nil, err
		}
	}
	originP := path.Join(dbif.config.GitRoot, originNs, originName)
	targetP := path.Join(dbif.config.GitRoot, targetNs, targetName)
	err = os.RemoveAll(targetP)
	if err != nil { return nil, err }
	originRepo, err := model.CreateLocalRepository(model.REPO_TYPE_GIT, originNs, originName, originP)
	if err != nil { return nil, err }
	targetRepo, err := model.CreateLocalForkOf(originRepo, targetNs, targetName, targetP)
	if err != nil { return nil, err }
	if err = tx.Commit(); err != nil { return nil, err }
	r, err := model.NewRepository(targetNs, targetName, targetRepo)
	r.Type = model.GetAegisType(targetRepo)
	r.Owner = owner
	if err != nil { return nil, err }
	return r, nil
}

func (dbif *SqliteAegisDatabaseInterface) UpdateRepositoryInfo(ns string, name string, robj *model.Repository) error {
	pfx := dbif.config.Database.TablePrefix
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid, repo_type FROM %srepository WHERE repo_namespace = ? AND repo_name = ?
`, pfx))
	if err != nil { return err }
	v := stmt1.QueryRow(ns, name)
	if v.Err() != nil { return v.Err() }
	var rowid string
	var repoType uint8
	err = v.Scan(&rowid, &repoType)
	if err != nil { return err }
	if len(rowid) <= 0 { return db.ErrEntityNotFound }
	tx, err := dbif.connection.Begin()
	if err != nil { tx.Rollback(); return err }
	defer tx.Rollback()
	stmt2, err := tx.Prepare(fmt.Sprintf(`
UPDATE %srepository
SET repo_description = ?, repo_owner = ?, repo_status = ?
WHERE rowid = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt2.Exec(robj.Description, robj.Owner, robj.Status, rowid)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	switch repoType {
	case model.REPO_TYPE_GIT:
		lgr := robj.Repository.(*gitlib.LocalGitRepository)
		lgr.Description = robj.Description
		// we don't deal with error here because it's not critical.
		lgr.SyncLocalDescription()
	}
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) UpdateRepositoryStatus(ns string, name string, newStatus model.AegisRepositoryStatus) error {
	pfx := dbif.config.Database.TablePrefix
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid FROM %srepository WHERE repo_namespace = ? AND repo_name = ?
`, pfx))
	if err != nil { return err }
	v := stmt1.QueryRow(ns, name)
	if v.Err() != nil { return v.Err() }
	var rowid string
	err = v.Scan(&rowid)
	if err != nil { return err }
	if len(rowid) <= 0 { return db.ErrEntityNotFound }
	tx, err := dbif.connection.Begin()
	if err != nil { tx.Rollback(); return err }
	stmt2, err := tx.Prepare(fmt.Sprintf(`
UPDATE %srepository
SET repo_status = ?
WHERE rowid = ?
`, pfx))
	if err != nil { tx.Rollback(); return err }
	_, err = stmt2.Exec(newStatus, rowid)
	if err != nil { tx.Rollback(); return err }
	err = tx.Commit()
	if err != nil { tx.Rollback(); return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) MoveRepository(oldNs string, oldName string, newNs string, newName string) error {
	// we first check if the new name is already taken
	pfx := dbif.config.Database.TablePrefix
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT 1 FROM %srepository
WHERE repo_namespace = ? AND repo_name = ?
`, pfx))
	if err != nil { return err }
	v := stmt1.QueryRow(newNs, newName)
	if v.Err() != nil { return v.Err() }
	var s string
	v.Scan(&s)
	if len(s) > 0 { return db.ErrEntityAlreadyExists }
	// this is sqlite thus we should be able to use rowid.
	// for other db engine we would need a PRIMARY KEY INT AUTO_INCREMENT.
	stmt2, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid FROM %srepository
WHERE repo_namespace = ? AND repo_name = ?
`, pfx))
	if err != nil { return err }
	v = stmt2.QueryRow(oldNs, oldName)
	if v.Err() != nil { return v.Err() }
	v.Scan(&s)
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	stmt3, err := tx.Prepare(fmt.Sprintf(`
UPDATE %srepository
SET repo_namespace = ?, repo_name = ?
WHERE rowid = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt3.Exec(newNs, newName, s)
	if err != nil { return err }
	oldPath := path.Join(dbif.config.GitRoot, oldNs, oldName)
	newPath := path.Join(dbif.config.GitRoot, newNs, newName)
	err = os.Rename(oldPath, newPath)
	if err != nil { tx.Rollback(); return err }
	err = tx.Commit()
	if err != nil { tx.Rollback(); return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) HardDeleteRepository(ns string, name string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	stmt, err := tx.Prepare(fmt.Sprintf(`
DELETE FROM %srepository
WHERE repo_namespace = ? AND repo_name = ?
`, pfx))
	if err != nil { tx.Rollback(); return err }
	_, err = stmt.Exec(ns, name)
	if err != nil { tx.Rollback(); return err }
	p := path.Join(dbif.config.GitRoot, ns, name)
	err = os.RemoveAll(p)
	if err != nil { tx.Rollback(); return err }
	err = tx.Commit()
	if err != nil { tx.Rollback(); return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) GetAllUsers(pageNum int, pageSize int) ([]*model.AegisUser, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT user_name, user_title, user_email, user_bio, user_website, user_status, user_password_hash
FROM %suser
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt.Close()
	r, err := stmt.Query(pageSize, pageNum * pageSize)
	if err != nil { return nil, err }
	defer r.Close()
	res := make([]*model.AegisUser, 0)
	var username, title, email, bio, website, ph string
	var status int
	for r.Next() {
		err = r.Scan(&username, &title, &email, &bio, &website, &status, &ph)
		if err != nil { return nil, err }
		res = append(res, &model.AegisUser{
			Name: username,
			Title: title,
			Email: email,
			Bio: bio,
			Website: website,
			Status: model.AegisUserStatus(status),
			PasswordHash: ph,
		})
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) GetAllNamespaces(pageNum int, pageSize int) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %snamespace
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt.Close()
	r, err := stmt.Query(pageSize, pageNum * pageSize)
	if err != nil { return nil, err }
	defer r.Close()
	res := make(map[string]*model.Namespace, 0)
	var name, title, desc, email, owner, acl string
	var reg_date int64
	var status int
	for r.Next() {
		err = r.Scan(&name, &title, &desc, &email, &owner, &reg_date, &status, &acl)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		res[name] = &model.Namespace{
			Name: name,
			Title: title,
			Description: desc,
			Email: email,
			Owner: owner,
			RegisterTime: reg_date,
			ACL: a,
			Status: model.AegisNamespaceStatus(status),
		}
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) GetAllRepositories(pageNum int, pageSize int) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_acl, repo_owner, repo_status, repo_fork_origin_namespace, repo_fork_origin_name
FROM %srepository
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt.Close()
	r, err := stmt.Query(pageSize, pageNum * pageSize)
	if err != nil { return nil, err }
	defer r.Close()
	res := make([]*model.Repository, 0)
	var ns, name, desc, acl, owner, forkOriginName, forkOriginNs string
	var status int
	var repoType uint8
	for r.Next() {
		err = r.Scan(&repoType, &ns, &name, &desc, &acl, &owner, &status, &forkOriginNs, &forkOriginName)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		p := path.Join(dbif.config.GitRoot, ns, name)
		lr, err := model.CreateLocalRepository(repoType, ns, name, p)
		if err != nil { return nil, err }
		res = append(res, &model.Repository{
			Type: repoType,
			Namespace: ns,
			Name: name,
			Owner: owner,
			Description: desc,
			AccessControlList: a,
			Status: model.AegisRepositoryStatus(status),
			Repository: lr,
			ForkOriginNamespace: forkOriginNs,
			ForkOriginName: forkOriginName,
		})
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) CountAllUser() (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(
		fmt.Sprintf(`SELECT COUNT(*) FROM %suser`, pfx),
	)
	if err != nil { return 0, err }
	defer stmt.Close()
	r := stmt.QueryRow()
	if r.Err() != nil { return 0, r.Err() }
	var res int64
	err = r.Scan(&res)
	if err != nil { return 0, r.Err() }
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) CountAllNamespace() (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(
		fmt.Sprintf(`SELECT COUNT(*) FROM %snamespace`, pfx),
	)
	if err != nil { return 0, err }
	defer stmt.Close()
	r := stmt.QueryRow()
	if r.Err() != nil { return 0, r.Err() }
	var res int64
	err = r.Scan(&res)
	if err != nil { return 0, r.Err() }
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) CountAllRepositoriesSearchResult(q string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	searchPattern := strings.ReplaceAll(q, "\\", "\\\\")
	searchPattern = strings.ReplaceAll(searchPattern, "%", "\\%")
	searchPattern = strings.ReplaceAll(searchPattern, "_", "\\_")
	searchPattern = "%" + searchPattern + "%"
	stmt, err := dbif.connection.Prepare(
		fmt.Sprintf(`
SELECT COUNT(*) FROM %srepository
WHERE (repo_name LIKE ? ESCAPE ? OR repo_namespace LIKE ? ESCAPE ?)`, pfx),
	)
	if err != nil { return 0, err }
	defer stmt.Close()
	r := stmt.QueryRow(searchPattern, "\\", searchPattern, "\\")
	if r.Err() != nil { return 0, r.Err() }
	var res int64
	err = r.Scan(&res)
	if err != nil { return 0, r.Err() }
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) CountAllRepositories() (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(
		fmt.Sprintf(`SELECT COUNT(*) FROM %srepository`, pfx),
	)
	if err != nil { return 0, err }
	defer stmt.Close()
	r := stmt.QueryRow()
	if r.Err() != nil { return 0, r.Err() }
	var res int64
	err = r.Scan(&res)
	if err != nil { return 0, r.Err() }
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) SearchForUser(k string, pageNum int, pageSize int) ([]*model.AegisUser, error) {
	pfx := dbif.config.Database.TablePrefix
	pattern := strings.ReplaceAll(k, "\\", "\\\\")
	pattern = strings.ReplaceAll(pattern, "%", "\\%")
	pattern = strings.ReplaceAll(pattern, "_", "\\_")
	pattern = "%" + pattern + "%"
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT user_name, user_title, user_email, user_bio, user_website, user_status, user_password_hash
FROM %suser
WHERE user_name LIKE ? ESCAPE ? OR user_title LIKE ? ESCAPE ?
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt.Close()
	r, err := stmt.Query(pattern, "\\", pattern, "\\", pageSize, pageNum * pageSize)
	if err != nil { return nil, err }
	defer r.Close()
	res := make([]*model.AegisUser, 0)
	var username, title, email, bio, website, ph string
	var status int
	for r.Next() {
		err = r.Scan(&username, &title, &email, &bio, &website, &status, &ph)
		if err != nil { return nil, err }
		res = append(res, &model.AegisUser{
			Name: username,
			Title: title,
			Email: email,
			Bio: bio,
			Website: website,
			Status: model.AegisUserStatus(status),
			PasswordHash: ph,
		})
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) SearchForNamespace(k string, pageNum int, pageSize int) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	pattern := strings.ReplaceAll(k, "\\", "\\\\")
	pattern = strings.ReplaceAll(pattern, "%", "\\%")
	pattern = strings.ReplaceAll(pattern, "_", "\\_")
	pattern = "%" + pattern + "%"
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %snamespace
WHERE ns_name LIKE ? ESCAPE ? OR ns_title LIKE ? ESCAPE ?
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt.Close()
	r, err := stmt.Query(pattern, "\\", pattern, "\\", pageSize, pageNum * pageSize)
	if err != nil { return nil, err }
	defer r.Close()
	res := make(map[string]*model.Namespace, 0)
	var name, title, desc, email, owner, acl string
	var reg_date int64
	var status int
	for r.Next() {
		err = r.Scan(&name, &title, &desc, &email, &owner, &reg_date, &status, &acl)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		res[name] = &model.Namespace{
			Name: name,
			Title: title,
			Description: desc,
			Email: email,
			Owner: owner,
			RegisterTime: reg_date,
			ACL: a,
			Status: model.AegisNamespaceStatus(status),
		}
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) SearchForRepository(k string, pageNum int, pageSize int) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	pattern := strings.ReplaceAll(k, "\\", "\\\\")
	pattern = strings.ReplaceAll(pattern, "%", "\\%")
	pattern = strings.ReplaceAll(pattern, "_", "\\_")
	pattern = "%" + pattern + "%"
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_acl, repo_owner, repo_status, repo_fork_origin_name, repo_fork_origin_namespace
FROM %srepository
WHERE (repo_namespace LIKE ? ESCAPE ? OR repo_name LIKE ? ESCAPE ?)
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt.Close()
	r, err := stmt.Query(pattern, "\\", pattern, "\\", pageSize, pageNum * pageSize)
	if err != nil { return nil, err }
	defer r.Close()
	res := make([]*model.Repository, 0)
	var ns, name, desc, acl, owner, forkOriginName, forkOriginNs string
	var status int
	var repoType uint8
	for r.Next() {
		err = r.Scan(&repoType, &ns, &name, &desc, &acl, &owner, &status, &forkOriginName, &forkOriginNs)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		p := path.Join(dbif.config.GitRoot, ns, name)
		lr, err := model.CreateLocalRepository(repoType, ns, name, p)
		if err != nil { return nil, err }
		res = append(res, &model.Repository{
			Type: repoType,
			Namespace: ns,
			Name: name,
			Description: desc,
			AccessControlList: a,
			Status: model.AegisRepositoryStatus(status),
			Repository: lr,
			ForkOriginNamespace: forkOriginNs,
			ForkOriginName: forkOriginName,
		})
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) SetNamespaceACL(nsName string, targetUserName string, aclt *model.ACLTuple) error {
	pfx := dbif.config.Database.TablePrefix
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_acl FROM %snamespace WHERE ns_name = ?
`, pfx))
	if err != nil { return err }
	defer stmt1.Close()
	r := stmt1.QueryRow(nsName)
	if r.Err() != nil { stmt1.Close(); return r.Err() }
	var aclStr string
	err = r.Scan(&aclStr)
	if err != nil { stmt1.Close(); return err }
	acl, err := model.ParseACL(aclStr)
	if err != nil { return err }
	if acl == nil {
		acl = &model.ACL{
			Version: "0",
			ACL: make(map[string]*model.ACLTuple, 0),
		}
	}
	acl.ACL[targetUserName] = aclt
	aclStr, err = acl.SerializeACL()
	if err != nil { return err }
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt2, err := tx.Prepare(fmt.Sprintf(`
UPDATE %snamespace SET ns_acl = ? WHERE ns_name = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt2.Exec(aclStr, nsName)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) SetRepositoryACL(nsName string, repoName string, targetUserName string, aclt *model.ACLTuple) error {
	pfx := dbif.config.Database.TablePrefix
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_acl FROM %srepository WHERE repo_namespace = ? AND repo_name = ?
`, pfx))
	if err != nil { return err }
	defer stmt1.Close()
	r := stmt1.QueryRow(nsName, repoName)
	if r.Err() != nil { stmt1.Close(); return r.Err() }
	var aclStr string
	err = r.Scan(&aclStr)
	if err != nil { stmt1.Close(); return err }
	acl, err := model.ParseACL(aclStr)
	if err != nil { return err }
	if acl == nil {
		acl = &model.ACL{
			Version: "0",
			ACL: make(map[string]*model.ACLTuple, 0),
		}
	}
	acl.ACL[targetUserName] = aclt
	aclStr, err = acl.SerializeACL()
	if err != nil { return err }
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt2, err := tx.Prepare(fmt.Sprintf(`
UPDATE %srepository SET repo_acl = ? WHERE repo_namespace = ? AND repo_name = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt2.Exec(aclStr, nsName, repoName)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) GetAllComprisingNamespace(username string) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	pattern := strings.ReplaceAll(username, "\\", "\\\\")
	pattern = strings.ReplaceAll(pattern, "%", "\\%")
	pattern = strings.ReplaceAll(pattern, "_", "\\_")
	pattern = "%" + pattern + "%"
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %snamespace
WHERE ns_owner = ? OR ns_acl LIKE ? ESCAPE ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt1.Close()
	r, err := stmt1.Query(username, pattern, "\\")
	if err != nil { return nil, err }
	defer r.Close()
	res := make(map[string]*model.Namespace, 0)
	for r.Next() {
		var name, title, desc, email, owner, acl string
		var regtime int64
		var status int64
		err = r.Scan(&name, &title, &desc, &email, &owner, &regtime, &status, &acl)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		res[name] = &model.Namespace{
			Name: name,
			Title: title,
			Description: desc,
			Email: email,
			Owner: owner,
			RegisterTime: regtime,
			ACL: a,
			Status: model.AegisNamespaceStatus(status),
		}
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) GetAllVisibleNamespacePaginated(username string, pageNum int, pageSize int) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	privateSelectClause := ""
	if len(username) > 0 {
		privateSelectClause = fmt.Sprintf("OR (ns_owner = ? OR ns_acl LIKE ? ESCAPE ?)", )
	}
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %snamespace
WHERE ns_status = 1 %s
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx, privateSelectClause))
	if err != nil { return nil, err }
	defer stmt1.Close()
	var rs *sql.Rows
	if len(username) > 0 {
		pattern := strings.ReplaceAll(username, "\\", "\\\\")
		pattern = strings.ReplaceAll(pattern, "%", "\\%")
		pattern = strings.ReplaceAll(pattern, "_", "\\_")
		pattern = "%" + pattern + "%"
		rs, err = stmt1.Query(username, pattern, "\\", pageSize, pageNum*pageSize)
	} else {
		rs, err = stmt1.Query(pageSize, pageNum*pageSize)
	}
	if err != nil { return nil, err }
	defer rs.Close()
	res := make(map[string]*model.Namespace, 0)
	for rs.Next() {
		var name, title, desc, email, owner, acl string
		var regtime int64
		var status int64
		err = rs.Scan(&name, &title, &desc, &email, &owner, &regtime, &status, &acl)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		res[name] = &model.Namespace{
			Name: name,
			Title: title,
			Description: desc,
			Email: email,
			Owner: owner,
			RegisterTime: regtime,
			ACL: a,
			Status: model.AegisNamespaceStatus(status),
		}
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) GetAllVisibleRepositoryPaginated(username string, pageNum int, pageSize int) ([]*model.Repository, error) {
	// private ns, not ns member, not repo member --> invisible.
	// private ns, ns member --> all repo visible.
	// private ns, not ns member, repo member --> only repo is visible.
	// public ns --> all repo visible.
	// ------>
	// select repo from all public ns
	// select repo from all ns member
	// select repo from all repo member
	pfx := dbif.config.Database.TablePrefix
	nsPrivateClause := ""
	repoPrivateClause := ""
	if len(username) > 0 {
		nsPrivateClause = "OR (ns_owner = ? OR ns_acl LIKE ? ESCAPE ?)"
		repoPrivateClause = "OR (repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)"
	}
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name
FROM %srepository
INNER JOIN (SELECT ns_name FROM %snamespace WHERE ns_status = 1 %s) a
ON %srepository.repo_namespace = a.ns_name
WHERE repo_status = 1 OR repo_status = 4 %s
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx, pfx, nsPrivateClause, pfx, repoPrivateClause))
	defer stmt.Close()
	if err != nil { return nil, err }
	var rs *sql.Rows
	if len(username) > 0 {
		pattern := ToSqlSearchPattern(username)
		rs, err = stmt.Query(username, pattern, "\\", username, pattern, "\\", pageSize, pageNum*pageSize)
	} else {
		rs, err = stmt.Query(pageSize, pageNum*pageSize)
	}
	if err != nil { return nil, err }
	defer rs.Close()
	res := make([]*model.Repository, 0)
	for rs.Next() {
		var ns, name, desc, owner, acl, forkOriginNs, forkOriginName string
		var status int64
		var repoType uint8
		err = rs.Scan(&repoType, &ns, &name, &desc, &owner, &acl, &status, &forkOriginNs, &forkOriginName)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		p := path.Join(dbif.config.GitRoot, ns, name)
		lr, err := model.CreateLocalRepository(repoType, ns, name, p)
		if err != nil { return nil, err }
		res = append(res, &model.Repository{
			Type: repoType,
			Namespace: ns,
			Name: name,
			Owner: owner,
			Description: desc,
			AccessControlList: a,
			Status: model.AegisRepositoryStatus(status),
			Repository: lr,
			ForkOriginNamespace: forkOriginNs,
			ForkOriginName: forkOriginName,
		})
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) CountAllVisibleNamespace(username string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	privateSelectClause := ""
	if len(username) > 0 {
		privateSelectClause = fmt.Sprintf("OR (ns_owner = ? OR ns_acl LIKE ? ESCAPE ?)", )
	}
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %snamespace WHERE ns_status = 1 %s
`, pfx, privateSelectClause))
	if err != nil { return 0, err }
	defer stmt1.Close()
	var r *sql.Row
	if len(username) > 0 {
		pattern := strings.ReplaceAll(username, "\\", "\\\\")
		pattern = strings.ReplaceAll(pattern, "%", "\\%")
		pattern = strings.ReplaceAll(pattern, "_", "\\_")
		pattern = "%" + pattern + "%"
		r = stmt1.QueryRow(username, pattern, "\\")
	} else {
		r = stmt1.QueryRow()
	}
	var res int64
	err = r.Scan(&res)
	if err != nil { return 0, r.Err() }
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) CountAllVisibleRepositories(username string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	nsPrivateClause := ""
	repoPrivateClause := ""
	if len(username) > 0 {
		nsPrivateClause = "OR (ns_owner = ? OR ns_acl LIKE ? ESCAPE ?)"
		repoPrivateClause = "OR (repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)"
	}
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*)
FROM %srepository
INNER JOIN (SELECT ns_name FROM %snamespace WHERE ns_status = 1 %s) a
ON %srepository.repo_namespace = a.ns_name
WHERE repo_status = 1 OR repo_status = 4 %s
`, pfx, pfx, nsPrivateClause, pfx, repoPrivateClause))
	if err != nil { return 0, err }
	var r *sql.Row
	if len(username) > 0 {
		pattern := ToSqlSearchPattern(username)
		r = stmt.QueryRow(
			username, pattern, "\\",
			username, pattern, "\\",
		)
	} else {
		r = stmt.QueryRow()
	}
	var res int64
	err = r.Scan(&res)
	if err != nil { return 0, r.Err() }
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) SearchAllVisibleNamespacePaginated(username string, query string, pageNum int, pageSize int) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	queryPattern := ToSqlSearchPattern(query)
	privateSelectClause := ""
	if len(username) > 0 {
		privateSelectClause = fmt.Sprintf("OR (ns_owner = ? OR ns_acl LIKE ? ESCAPE ?)", )
	}
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %snamespace
WHERE
    (ns_name LIKE ? ESCAPE ? OR ns_title LIKE ? ESCAPE ?)
    AND (ns_status = 1 %s)
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx, privateSelectClause))
	if err != nil { return nil, err }
	defer stmt.Close()
	var r *sql.Rows
	if len(username) > 0 {
		usernamePattern := ToSqlSearchPattern(username)
		r, err = stmt.Query(queryPattern, "\\", queryPattern, "\\", username, usernamePattern, "\\", pageSize, pageNum * pageSize)
	} else {
		r, err = stmt.Query(queryPattern, "\\", queryPattern, "\\", pageSize, pageNum * pageSize)
	}
	if err != nil { return nil, err }
	defer r.Close()
	res := make(map[string]*model.Namespace, 0)
	var name, title, desc, email, owner, acl string
	var reg_date int64
	var status int
	for r.Next() {
		err = r.Scan(&name, &title, &desc, &email, &owner, &reg_date, &status, &acl)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		res[name] = &model.Namespace{
			Name: name,
			Title: title,
			Description: desc,
			Email: email,
			Owner: owner,
			RegisterTime: reg_date,
			ACL: a,
			Status: model.AegisNamespaceStatus(status),
		}
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) SearchAllVisibleRepositoryPaginated(username string, query string, pageNum int, pageSize int) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	queryPattern := ToSqlSearchPattern(query)
	nsPrivateClause := ""
	repoPrivateClause := ""
	if len(username) > 0 {
		nsPrivateClause = "OR (ns_owner = ? OR ns_acl LIKE ? ESCAPE ?)"
		repoPrivateClause = "OR (repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)"
	}
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name
FROM %srepository
INNER JOIN (SELECT ns_name FROM %snamespace WHERE ns_status = 1 %s) a
ON %srepository.repo_namespace = a.ns_name
WHERE
    (repo_name LIKE ? ESCAPE ? OR repo_namespace LIKE ? ESCAPE ?)
    AND (repo_status = 1 OR repo_status = 4 %s)
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx, pfx, nsPrivateClause, pfx, repoPrivateClause))
	if err != nil { return nil, err }
	defer stmt.Close()
	var r *sql.Rows
	if len(username) > 0 {
		usernamePattern := ToSqlSearchPattern(username)
		r, err = stmt.Query(
			username, usernamePattern, "\\",
			queryPattern, "\\",
			queryPattern, "\\",
			username, usernamePattern, "\\",
			pageSize, pageNum * pageSize,
		)
	} else {
		r, err = stmt.Query(
			queryPattern, "\\",
			queryPattern, "\\",
			pageSize, pageNum * pageSize,
		)
	}
	if err != nil { return nil, err }
	defer r.Close()
	res := make([]*model.Repository, 0)
	for r.Next() {
		var ns, name, desc, acl, forkOriginNamespace, forkOriginName string
		var status int64
		var repoType uint8
		err = r.Scan(&repoType, &ns, &name, &desc, &acl, &status, &forkOriginNamespace, &forkOriginName)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		p := path.Join(dbif.config.GitRoot, ns, name)
		lr, err := model.CreateLocalRepository(repoType, ns, name, p)
		if err != nil { return nil, err }
		res = append(res, &model.Repository{
			Type: repoType,
			Namespace: ns,
			Name: name,
			Description: desc,
			AccessControlList: a,
			Status: model.AegisRepositoryStatus(status),
			Repository: lr,
			ForkOriginNamespace: forkOriginNamespace,
			ForkOriginName: forkOriginName,
		})
	}
	return res, nil
}


func (dbif *SqliteAegisDatabaseInterface) CountAllVisibleNamespaceSearchResult(username string, pattern string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	searchPattern := ToSqlSearchPattern(pattern)
	privateSelectClause := ""
	if len(username) > 0 {
		privateSelectClause = fmt.Sprintf("AND (ns_owner = ? OR ns_acl LIKE ? ESCAPE ?)", )
	}
	stmt, err := dbif.connection.Prepare(
		fmt.Sprintf(`
SELECT COUNT(*) FROM %snamespace
WHERE (ns_name LIKE ? ESCAPE ? OR ns_title LIKE ? ESCAPE ?)
%s
`, pfx, privateSelectClause),
	)
	if err != nil { return 0, err }
	defer stmt.Close()
	var r *sql.Row
	if len(username) > 0 {
		usernamePattern := ToSqlSearchPattern(username)
		r = stmt.QueryRow(searchPattern, "\\", searchPattern, "\\", username, usernamePattern, "\\")
	} else {
		r = stmt.QueryRow(searchPattern, "\\", searchPattern, "\\")
	}
	if r.Err() != nil { return 0, r.Err() }
	var res int64
	err = r.Scan(&res)
	if err != nil { return 0, r.Err() }
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) CountAllVisibleRepositoriesSearchResult(username string, pattern string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	searchPattern := ToSqlSearchPattern(pattern)
	nsPrivateClause := ""
	repoPrivateClause := ""
	if len(username) > 0 {
		nsPrivateClause = "OR (ns_owner = ? OR ns_acl LIKE ? ESCAPE ?)"
		repoPrivateClause = "OR (repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)"
	}
	stmt, err := dbif.connection.Prepare(
		fmt.Sprintf(`
SELECT COUNT(*)
FROM %srepository
INNER JOIN (SELECT ns_name FROM %snamespace WHERE ns_status = 1 %s) a
ON %srepository.repo_namespace = a.ns_name
WHERE
    (repo_name LIKE ? ESCAPE ? OR repo_namespace LIKE ? ESCAPE ?)
    AND (repo_status = 1 OR repo_status = 4 %s)
`, pfx, pfx, nsPrivateClause, pfx, repoPrivateClause),
	)
	if err != nil { return 0, err }
	defer stmt.Close()
	var r *sql.Row
	
	if len(username) > 0 {
		usernamePattern := ToSqlSearchPattern(username)
		r = stmt.QueryRow(
			username, usernamePattern, "\\",
			searchPattern, "\\",
			searchPattern, "\\",
			username, usernamePattern, "\\",
		)
	} else {
		r = stmt.QueryRow(searchPattern, "\\", searchPattern, "\\")
	}
	if r.Err() != nil { return 0, r.Err() }
	var res int64
	err = r.Scan(&res)
	if err != nil { return 0, r.Err() }
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) GetAllRepositoryIssue(ns string, name string) ([]*model.Issue, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid, issue_id, issue_author, issue_status, issue_title, issue_content, issue_timestamp
FROM %sissue
WHERE repo_namespace = ? AND repo_name = ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt.Close()
	r, err := stmt.Query(ns, name)
	if err != nil { return nil, err }
	defer r.Close()
	res := make([]*model.Issue, 0)
	for r.Next() {
		var issueAbsId, issueTimestamp int64
		var issueId, issueStatus int
		var issueAuthor, issueTitle, issueContent string
		err = r.Scan(&issueAbsId, &issueId, &issueAuthor, &issueStatus, &issueTitle, &issueContent, &issueTimestamp)
		if err != nil { return nil, err }
		res = append(res, &model.Issue{
			IssueAbsId: issueAbsId,
			RepoNamespace: ns,
			RepoName: name,
			IssueStatus: issueStatus,
			IssueId: issueId,
			IssueTime: issueTimestamp,
			IssueTitle: issueTitle,
			IssueAuthor: issueAuthor,
			IssueContent: issueContent,
		})
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) GetRepositoryIssue(ns string, name string, iid int) (*model.Issue, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid, issue_timestamp, issue_author, issue_title, issue_content, issue_status
FROM %sissue
WHERE repo_namespace = ? AND repo_name = ? AND issue_id = ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt.Close()
	r := stmt.QueryRow(ns, name, iid)
	if r.Err() != nil { return nil, r.Err() }
	var absid, timestamp int64
	var status int
	var author, title, content string
	err = r.Scan(&absid, &timestamp, &author, &title, &content, &status)
	if err != nil { return nil, err }
	return &model.Issue{
		IssueAbsId: absid,
		RepoNamespace: ns,
		RepoName: name,
		IssueId: iid,
		IssueAuthor: author,
		IssueTitle: title,
		IssueTime: timestamp,
		IssueContent: content,
		IssueStatus: status,
	}, nil
}

func (dbif *SqliteAegisDatabaseInterface) CountAllRepositoryIssue(ns string, name string) (int, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*)
FROM %sissue
WHERE repo_namespace = ? AND repo_name = ?
`, pfx))
	if err != nil { return 0, err }
	defer stmt.Close()
	r := stmt.QueryRow(ns, name)
	if r.Err() != nil { return 0, r.Err() }
	var res int
	err = r.Scan(&res)
	if err != nil { return 0, err }
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) NewRepositoryIssue(ns string, name string, author string, title string, content string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %sissue WHERE repo_namespace = ? AND repo_name = ?
`, pfx))
	if err != nil { return 0, err }
	defer stmt.Close()
	r := stmt.QueryRow(ns, name)
	if r.Err() != nil { return 0, err }
	var res int64
	err = r.Scan(&res)
	if err != nil { return 0, err }
	res += 1
	tx, err := dbif.connection.Begin()
	if err != nil { return 0, err }
	defer tx.Rollback()
	stmt2, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %sissue(repo_namespace, repo_name, issue_id, issue_timestamp, issue_author, issue_title, issue_content, issue_status)
VALUES (?,?,?,?,?,?,?,?)
`, pfx))
	if err != nil { return 0, err }
	_, err = stmt2.Exec(ns, name, res, time.Now().Unix(), author, title, content, model.ISSUE_OPENED)
	if err != nil { return 0, err }
	err = tx.Commit()
	if err != nil { return 0, err }
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) HardDeleteRepositoryIssue(ns string, name string, issueId int) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
DELETE FROM %sissue WHERE repo_namespace = ? AND repo_name = ? AND issue_id = ?
`, pfx))
	if err != nil { return err }
	defer stmt.Close()
	_, err = stmt.Exec(ns, name, issueId)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) GetAllIssueEvent(ns string, name string, issueId int) ([]*model.IssueEvent, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid FROM %sissue WHERE repo_namespace = ? AND repo_name = ? AND issue_id = ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt1.Close()
	r := stmt1.QueryRow(ns, name, issueId)
	if r.Err() != nil { return nil, r.Err() }
	var absId int
	err = r.Scan(&absId)
	if err != nil { return nil, err }
	stmt2, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid, issue_event_type, issue_event_time, issue_event_author, issue_event_content
FROM %sissue_event
WHERE issue_abs_id = ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt2.Close()
	rs, err := stmt2.Query(absId)
	if err != nil { return nil, err }
	defer rs.Close()
	res := make([]*model.IssueEvent, 0)
	for rs.Next() {
		var author, content string
		var eventType int
		var eventAbsId, timestamp int64
		err = rs.Scan(&eventAbsId, &eventType, &timestamp, &author, &content)
		if err != nil { return nil, err }
		res = append(res, &model.IssueEvent{
			EventAbsId: eventAbsId,
			EventType: eventType,
			EventTimestamp: timestamp,
			EventAuthor: author,
			EventContent: content,
		})
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) NewRepositoryIssueEvent(ns string, name string, issueId int, eType int, author string, content string) error {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid, issue_status FROM %sissue WHERE repo_namespace = ? AND repo_name = ? AND issue_id = ?
`, pfx))
	if err != nil { return err }
	defer stmt.Close()
	r := stmt.QueryRow(ns, name, issueId)
	if r.Err() != nil { return r.Err() }
	var issueAbsId int64
	var issueStatus int
	err = r.Scan(&issueAbsId, &issueStatus)
	if err != nil { return err }
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt2, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %sissue_event(issue_abs_id, issue_event_type, issue_event_time, issue_event_author, issue_event_content) VALUES (?,?,?,?,?)
`, pfx))
	if err != nil { return err }
	defer stmt2.Close()
	_, err = stmt2.Exec(issueAbsId, eType, time.Now().Unix(), author, content)
	if err != nil { return err }
	newIssueStatus := issueStatus
	switch eType {
	case model.EVENT_CLOSED_AS_SOLVED:
		newIssueStatus = model.ISSUE_CLOSED_AS_SOLVED
	case model.EVENT_CLOSED_AS_DISCARDED:
		newIssueStatus = model.ISSUE_CLOSED_AS_DISCARDED
	case model.EVENT_REOPENED:
		newIssueStatus = model.ISSUE_OPENED
	}
	if newIssueStatus != issueStatus {
		stmt3, err := tx.Prepare(fmt.Sprintf(`
UPDATE %sissue SET issue_status = ? WHERE rowid = ?
`, pfx))
		if err != nil { return err }
		defer stmt3.Close()
		_, err = stmt3.Exec(newIssueStatus, issueAbsId)
		if err != nil { return err }
	}
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) HardDeleteRepositoryIssueEvent(eventAbsId int64) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
DELETE FROM %sissue_event WHERE rowid = ?
`, pfx))
	if err != nil { return err }
	defer stmt.Close()
	_, err = stmt.Exec(eventAbsId)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) GetAllBelongingNamespace(viewingUser string, userName string) ([]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	nsStatusClause := "ns_status = 1"
	if len(viewingUser) > 0 {
		if viewingUser == userName {
			nsStatusClause = "1"
		} else {
			nsStatusClause = fmt.Sprintf("ns_acl LIKE ? ESCAPE ?")
		}
	}
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %snamespace
WHERE (%s) AND (ns_owner = ? OR ns_acl LIKE ? ESCAPE ?)
`, pfx, nsStatusClause))
	if err != nil { return nil, err }
	defer stmt.Close()
	userSearchPattern := ToSqlSearchPattern(userName)
	var rs *sql.Rows
	if len(viewingUser) > 0 {
		if viewingUser == userName {
			rs, err = stmt.Query(userName, userSearchPattern, "\\")
		} else {
			viewingPattern := ToSqlSearchPattern(viewingUser)
			rs, err = stmt.Query(viewingPattern, "\\", userName, userSearchPattern, "\\")
		}
	} else {
		rs, err = stmt.Query(userName, userSearchPattern, "\\")
	}
	if err != nil { return nil, err }
	res := make([]*model.Namespace, 0)
	for rs.Next() {
		var name, title, desc, email, owner, acl string
		var regtime int64
		var status int64
		err = rs.Scan(&name, &title, &desc, &email, &owner, &regtime, &status, &acl)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		res = append(res, &model.Namespace{
			Name: name,
			Title: title,
			Description: desc,
			Email: email,
			Owner: owner,
			RegisterTime: regtime,
			ACL: a,
			Status: model.AegisNamespaceStatus(status),
		})
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) GetAllBelongingRepository(viewingUser string, user string, pageNum int, pageSize int) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	nsStatusClause := "ns_status = 1"
	repoStatusClause := "repo_status = 1 OR repo_status = 4"
	if len(viewingUser) > 0 {
		if viewingUser == user {
			nsStatusClause = "1"
			repoStatusClause = "1"
		} else {
			nsStatusClause = "ns_acl LIKE ? ESCAPE ?"
			repoStatusClause = "repo_status = 1 OR repo_status = 4 OR (repo_acl LIKE ? ESCAPE ?)"
		}
	}
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name
FROM %srepository
    INNER JOIN (SELECT ns_name FROM %snamespace WHERE (%s)) a
    ON %srepository.repo_namespace = a.ns_name
WHERE (%s) AND (repo_owner = ?)
`, pfx, pfx, nsStatusClause, pfx, repoStatusClause))
	if err != nil { return nil, err }
	defer stmt.Close()
	var r *sql.Rows
	if len(viewingUser) > 0 {
		if viewingUser == user {
			r, err = stmt.Query(user)
		} else {
			viewingPattern := ToSqlSearchPattern(viewingUser)
			r, err = stmt.Query(viewingPattern, "\\", viewingPattern, "\\", user)
		}
	} else {
		r, err = stmt.Query(user)
	}
	if err != nil { return nil, err }
	res := make([]*model.Repository, 0)
	for r.Next() {
		var ns, name, desc, acl, owner, forkOriginNamespace, forkOriginName string
		var status int64
		var repoType uint8
		err := r.Scan(&repoType, &ns, &name, &desc, &owner, &acl, &status, &forkOriginNamespace, &forkOriginName)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		p := path.Join(dbif.config.GitRoot, ns, name)
		lr, err := model.CreateLocalRepository(repoType, ns, name, p)
		if err != nil { return nil, err }
		res = append(res, &model.Repository{
			Type: repoType,
			Namespace: ns,
			Name: name,
			Description: desc,
			Owner: owner,
			AccessControlList: a,
			Status: model.AegisRepositoryStatus(status),
			Repository: lr,
			ForkOriginNamespace: forkOriginNamespace,
			ForkOriginName: forkOriginName,
		})
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) GetForkRepositoryOfUser(username string, originNamespace string, originName string) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_acl, repo_status
FROM %srepository
WHERE repo_owner = ? AND repo_fork_origin_namespace = ? AND repo_fork_origin_name = ?
`, pfx))
	if err != nil { return nil, err }
	r, err := stmt.Query(username, originNamespace, originName)
	if err != nil { return nil, err }
	defer r.Close()
	res := make([]*model.Repository, 0)
	for r.Next() {
		var ns, name, desc, acl string
		var status int
		var repoType uint8
		err = r.Scan(&repoType, &ns, &name, &desc, &acl, &status)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, nil
			}
			return nil, err
		}
		p := path.Join(dbif.config.GitRoot, ns, name)
		lr, err := model.CreateLocalRepository(repoType, ns, name, p)
		if err != nil { return nil, err }
		mr, err := model.NewRepository(ns, name, lr)
		mr.Owner = username
		mr.Type = repoType
		mr.Status = model.AegisRepositoryStatus(status)
		mr.ForkOriginNamespace = originNamespace
		mr.ForkOriginName = originName
		aclobj, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		mr.AccessControlList = aclobj
		res = append(res, mr)
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) GetAllPullRequestPaginated(namespace string, name string, pageNum int, pageSize int) ([]*model.PullRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid, pull_request_id, username, title, receiver_branch, provider_namespace, provider_name, provider_branch, merge_conflict_check_result, merge_conflict_check_timestamp, pull_request_status, pull_request_timestamp
FROM %spull_request
WHERE receiver_namespace = ? AND receiver_name = ?
ORDER BY pull_request_id ASC LIMIT ? OFFSET ?
`, pfx))
	if err != nil { return nil, err }
	r, err := stmt.Query(namespace, name, pageSize, pageNum*pageSize)
	if err != nil { return nil, err }
	res := make([]*model.PullRequest, 0)
	for r.Next() {
		var prid, absid, prtime int64
		var status int
		var username, title, receiverBranch string
		var providerNamespace, providerName, provideBranch string
		var mergeCheckResultString string
		var mergeCheckTimestamp int64
		err = r.Scan(&absid, &prid, &username, &title, &receiverBranch, &providerNamespace, &providerName, &provideBranch, &mergeCheckResultString, &mergeCheckTimestamp, &status, &prtime)
		if err != nil { return nil, err }
		var mergeCheckResult *gitlib.MergeCheckResult = nil
		if len(mergeCheckResultString) > 0 {		
			err = json.Unmarshal([]byte(mergeCheckResultString), &mergeCheckResult)
			if err != nil { return nil, err }
		}
		res = append(res, &model.PullRequest{
			PRId: prid,
			PRAbsId: absid,
			Title: title,
			Author: username,
			Timestamp: prtime,
			ReceiverNamespace: namespace,
			ReceiverName: name,
			ReceiverBranch: receiverBranch,
			ProviderNamespace: providerNamespace,
			ProviderName: providerName,
			ProviderBranch: provideBranch,
			Status: status,
			MergeCheckResult: mergeCheckResult,
			MergeCheckTimestamp: mergeCheckTimestamp,
		})
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) CountIssue(query string, namespace string, name string, filterType int) (int, error) {
	pfx := dbif.config.Database.TablePrefix
	statusClause := ""
	switch filterType {
    case 1: statusClause = "issue_status = 1"
	case 2: statusClause = "NOT (issue_status = 1)"
	case 3: statusClause = "issue_status = 2"
	case 4: statusClause = "issue_status = 3"
	}
	queryClause := ""
	if query == "" {
		queryClause = "issue_title LIKE ? ESCAPE ?"
	}
	condition := ""
	if statusClause == "" {
		if queryClause == "" {
		} else {
			condition = fmt.Sprintf("WHERE %s", queryClause)
		}
	} else {
		if queryClause == "" {
			condition = fmt.Sprintf("WHERE %s", statusClause)
		} else {
			condition = fmt.Sprintf("WHERE %s AND %s", queryClause, statusClause)
		}
	}
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %sissue %s
`, pfx, condition))
	if err != nil { return 0, err }
	var cnt int
	var r *sql.Row
	if queryClause == "" {
		r = stmt1.QueryRow()
	} else {
		queryPattern := ToSqlSearchPattern(query)
		r = stmt1.QueryRow(queryPattern, "\\")
	}
	if r.Err() != nil { return 0, r.Err() }
	err = r.Scan(&cnt)
	if err != nil { return 0, err }
	return cnt, nil
}

func (dbif *SqliteAegisDatabaseInterface) SearchIssuePaginated(query string, namespace string, name string, filterType int, pageNum int, pageSize int) ([]*model.Issue, error) {
	pfx := dbif.config.Database.TablePrefix
	statusClause := ""
	switch filterType {
    case 1: statusClause = "AND issue_status = 1"
	case 2: statusClause = "AND NOT (issue_status = 1)"
	case 3: statusClause = "AND issue_status = 2"
	case 4: statusClause = "AND issue_status = 3"
	}
	queryClause := ""
	if query != "" {
		queryClause = "AND issue_title LIKE ? ESCAPE ?"
	}
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid, issue_id, issue_author, issue_status, issue_title, issue_content, issue_timestamp
FROM %sissue
WHERE repo_namespace = ? AND repo_name = ? %s %s
ORDER BY issue_timestamp DESC LIMIT ? OFFSET ?
`, pfx, statusClause, queryClause))
	if err != nil { return nil, err }
	var r *sql.Rows
	if queryClause == "" {
		r, err = stmt1.Query(namespace, name, pageSize, pageNum*pageSize)
	} else {
		queryPattern := ToSqlSearchPattern(query)
		r, err = stmt1.Query(namespace, name, queryPattern, "\\", pageSize, pageNum*pageSize)
	}
	if err != nil { return nil, err }
	res := make([]*model.Issue, 0)
	for r.Next() {
		var issueAbsId, issueTimestamp int64
		var issueId, issueStatus int
		var issueAuthor, issueTitle, issueContent string
		err = r.Scan(&issueAbsId, &issueId, &issueAuthor, &issueStatus, &issueTitle, &issueContent, &issueTimestamp)
		if err != nil { return nil, err }
		res = append(res, &model.Issue{
			IssueAbsId: issueAbsId,
			RepoNamespace: namespace,
			RepoName: name,
			IssueStatus: issueStatus,
			IssueId: issueId,
			IssueTime: issueTimestamp,
			IssueTitle: issueTitle,
			IssueAuthor: issueAuthor,
			IssueContent: issueContent,
		})
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) NewPullRequest(username string, title string, receiverNamespace string, receiverName string, receiverBranch string, providerNamespace string, providerName string, providerBranch string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %spull_request
WHERE receiver_namespace = ? AND receiver_name = ?
`, pfx))
	if err != nil { return 0, err }
	var newid int64
	err = stmt1.QueryRow(receiverNamespace, receiverName).Scan(&newid)
	newid += 1
	if err != nil { return 0, err }
	tx, err := dbif.connection.Begin()
	if err != nil { return 0, err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %spull_request(
    username, pull_request_id, title,
    receiver_namespace, receiver_name, receiver_branch,
    provider_namespace, provider_name, provider_branch,
    merge_conflict_check_result, merge_conflict_check_timestamp,
    pull_request_status, pull_request_timestamp
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
`, pfx))
	if err != nil { return 0, err }
	_, err = stmt.Exec(
		username, newid, title,
		receiverNamespace, receiverName, receiverBranch,
		providerNamespace, providerName, providerBranch,
		new(string), 0,
		model.PULL_REQUEST_OPEN, time.Now().Unix(),
	)
	if err != nil { return 0, err }
	err = tx.Commit()
	if err != nil { return 0, err }
	return newid, nil
}

func (dbif *SqliteAegisDatabaseInterface) GetPullRequest(namespace string, name string, id int64) (*model.PullRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid, username, title, receiver_branch, provider_namespace, provider_name, provider_branch, merge_conflict_check_result, merge_conflict_check_timestamp, pull_request_status, pull_request_timestamp
FROM %spull_request
WHERE receiver_namespace = ? AND receiver_name = ? AND pull_request_id = ?
`, pfx))
	if err != nil { return nil, err }
	r := stmt.QueryRow(namespace, name, id)
	if r.Err() != nil { return nil, r.Err() }
	var rowid, prtime, mchtime int64
	var username, title, receiverBranch string
	var providerNamespace, providerName, providerBranch string
	var mchResult string
	var prstatus int
	err = r.Scan(&rowid, &username, &title, &receiverBranch, &providerNamespace, &providerName, &providerBranch, &mchResult, &mchtime, &prstatus, &prtime)
	if err != nil { return nil, err }
	var mergeCheckResult *gitlib.MergeCheckResult = nil
	if len(mchResult) > 0 {
		err = json.Unmarshal([]byte(mchResult), &mergeCheckResult)
		if err != nil { return nil, err }
	}
	return &model.PullRequest{
		PRId: id,
		PRAbsId: rowid,
		Title: title,
		Author: username,
		ReceiverNamespace: namespace,
		ReceiverName: name,
		ReceiverBranch: receiverBranch,
		ProviderNamespace: providerNamespace,
		ProviderName: providerName,
		ProviderBranch: providerBranch,
		MergeCheckResult: mergeCheckResult,
		MergeCheckTimestamp: mchtime,
		Status: prstatus,
		Timestamp: prtime,
	}, nil
}

func (dbif *SqliteAegisDatabaseInterface) GetPullRequestByAbsId(absId int64) (*model.PullRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT username, pull_request_id, title, receiver_namespace, receiver_name, receiver_branch, provider_namespace, provider_name, provider_branch, merge_conflict_check_result, merge_conflict_check_timestamp, pull_request_status, pull_request_timestamp
FROM %spull_request
WHERE rowid = ?
`, pfx))
	if err != nil { return nil, err }
	r := stmt.QueryRow(absId)
	if r.Err() != nil { return nil, r.Err() }
	var prid, prtime, mchtime int64
	var username, title, receiverNamespace, receiverName, receiverBranch string
	var providerNamespace, providerName, providerBranch string
	var mchResult string
	var prstatus int
	err = r.Scan(&username, &prid, &title, &receiverNamespace, &receiverName, &receiverBranch, &providerNamespace, &providerName, &providerBranch, &mchResult, &mchtime, &prstatus, &prtime)
	if err != nil { return nil, err }
	var mergeCheckResult gitlib.MergeCheckResult
	err = json.Unmarshal([]byte(mchResult), &mergeCheckResult)
	if err != nil { return nil, err }
	return &model.PullRequest{
		PRId: prid,
		PRAbsId: absId,
		Title: title,
		ReceiverNamespace: receiverNamespace,
		ReceiverName: receiverName,
		ReceiverBranch: receiverBranch,
		ProviderNamespace: providerNamespace,
		ProviderName: providerName,
		ProviderBranch: providerBranch,
		MergeCheckResult: &mergeCheckResult,
		MergeCheckTimestamp: mchtime,
		Status: prstatus,
		Timestamp: prtime,
	}, nil
}

func (dbif *SqliteAegisDatabaseInterface) CheckPullRequestMergeConflict(absId int64) (*gitlib.MergeCheckResult, error) {
	// WARNING: currently only works when when the source &
	// the target is git repo. currently (2025.7.28) this check
	// is performed at the controller side, i.e. users cannot
	// create pull request if the repo is not git repo, but the
	// code can still be called. DO NOT CALL UNLESS YOU KNOW
	// WHAT YOU'RE DOING.
	// TODO: fix this after figuring things out.
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT receiver_namespace, receiver_name, receiver_branch, provider_namespace, provider_name, provider_branch
FROM %spull_request
WHERE rowid = ?
`, pfx))
	if err != nil { return nil, err }
	r := stmt.QueryRow(absId)
	if r.Err() != nil { return nil, r.Err() }
	var receiverNamespace, receiverName, receiverBranch string
	var providerNamespace, providerName, providerBranch string
	err = r.Scan(&receiverNamespace, &receiverName, &receiverBranch, &providerNamespace, &providerName, &providerBranch)
	if err != nil { return nil, err }
	tx, err := dbif.connection.Begin()
	if err != nil { return nil, err }
	defer tx.Rollback()
	p := path.Join(dbif.config.GitRoot, receiverNamespace, receiverName)
	lgr := gitlib.NewLocalGitRepository(receiverNamespace, receiverName, p)
	remoteName := fmt.Sprintf("%s/%s", providerNamespace, providerName)
	mr, err := lgr.CheckBranchMergeConflict(receiverBranch, remoteName, providerBranch)
	if err != nil { return nil, err }
	stmt2, err := tx.Prepare(fmt.Sprintf(`
UPDATE %spull_request
SET merge_conflict_check_result = ?, merge_conflict_check_timestamp = ?
WHERE rowid = ?
`, pfx))
	if err != nil { return nil, err }
	mrstr, err := json.Marshal(mr)
	if err != nil { return nil, err }
	_, err = stmt2.Exec(string(mrstr), time.Now().Unix(), absId)
	if err != nil { return nil, err }
	err = tx.Commit()
	if err != nil { return nil, err }
	return mr, nil
}

func (dbif *SqliteAegisDatabaseInterface) DeletePullRequest(absId int64) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
DELETE FROM %spull_request WHERE rowid = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt.Exec(absId)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) GetAllPullRequestEventPaginated(absId int64, pageNum int, pageSize int) ([]*model.PullRequestEvent, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT event_type, event_timestamp, event_author, event_content
FROM %spull_request_event
WHERE pull_request_abs_id = ?
ORDER BY event_timestamp ASC LIMIT ? OFFSET ?
`, pfx))
	if err != nil { return nil, err }
	res := make([]*model.PullRequestEvent, 0)
	r, err := stmt.Query(absId, pageSize, pageNum*pageSize)
	if err != nil { return nil, err }
	for r.Next() {
		var eventType int
		var eventTime int64
		var eventAuthor, eventContent string
		err = r.Scan(&eventType, &eventTime, &eventAuthor, &eventContent)
		if err != nil { return nil, err }
		res = append(res, &model.PullRequestEvent{
			PRAbsId: absId,
			EventType: eventType,
			EventTimestamp: eventTime,
			EventAuthor: eventAuthor,
			EventContent: eventContent,
		})
	}
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) CheckAndMergePullRequest(absId int64, username string) error {
	// WARNING: currently only works when when the source &
	// the target is git repo. currently (2025.7.28) this check
	// is performed at the controller side, i.e. users cannot
	// create pull request if the repo is not git repo, but the
	// code can still be called. DO NOT CALL UNLESS YOU KNOW
	// WHAT YOU'RE DOING.
	// TODO: fix this after figuring things out.
	r, err := dbif.CheckPullRequestMergeConflict(absId)
	if err != nil { return err }
	// TODO: this would need to be fixed in the future...
	if !r.Successful { return nil }
	pfx := dbif.config.Database.TablePrefix
	stmt0, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT user_email, user_title FROM %suser WHERE user_name = ?
`, pfx))
	if err != nil { return err }
	rr := stmt0.QueryRow(username)
	if rr.Err() != nil { return rr.Err() }
	var email, userTitle string
	err = rr.Scan(&email, &userTitle)
	if err != nil { return err }
	// fetch
	buf := new(bytes.Buffer)
	cmd1 := exec.Command("git", "fetch", r.ProviderRemoteName, r.ProviderBranch)
	cmd1.Dir = r.ReceiverLocation
	cmd1.Stderr = buf
	err = cmd1.Run()
	if err != nil { return errors.New(err.Error() + ": " + buf.String()) }
	buf.Reset()
	providerFullName := fmt.Sprintf("%s/%s", r.ProviderRemoteName, r.ProviderBranch)
	cmd2 := exec.Command("git", "merge-tree", "--write-tree", r.ReceiverBranch, providerFullName)
	cmd2.Dir = r.ReceiverLocation
	cmd2.Stdout = buf
	err = cmd2.Run()
	if err != nil { return fmt.Errorf("Failed while merge-tree: %s", err.Error()) }
	treeId := strings.TrimSpace(buf.String())
	mergeMessage := fmt.Sprintf("merge: from %s/%s to %s", r.ProviderRemoteName, r.ProviderBranch, r.ReceiverBranch)
	buf.Reset()
	cmd3 := exec.Command("git", "commit-tree", treeId, "-m", mergeMessage, "-p", r.ReceiverBranch, "-p", providerFullName)
	cmd3.Dir = r.ReceiverLocation
	cmd3.Stdout = buf
	cmd3.Env = os.Environ()
	cmd3.Env = append(cmd3.Env, fmt.Sprintf("GIT_AUTHOR_NAME=%s", userTitle))
	cmd3.Env = append(cmd3.Env, fmt.Sprintf("GIT_AUTHOR_EMAIL=%s", email))
	cmd3.Env = append(cmd3.Env, fmt.Sprintf("GIT_COMMITTER_NAME=%s", userTitle))
	cmd3.Env = append(cmd3.Env, fmt.Sprintf("GIT_COMMITTER_EMAIL=%s", email))
	err = cmd3.Run()
	if err != nil { return fmt.Errorf("Failed while commit-tree: %s", err.Error()) }
	commitId := strings.TrimSpace(buf.String())
	buf.Reset()
	receiverBranchFullName := fmt.Sprintf("refs/heads/%s", r.ReceiverBranch)
	cmd4 := exec.Command("git", "update-ref", receiverBranchFullName, commitId)
	cmd4.Dir = r.ReceiverLocation
	cmd4.Stderr = buf
	err = cmd4.Run()
	if err != nil { return fmt.Errorf("Failed while update-ref: %s; %s", err.Error(), buf.String()) }
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	t := time.Now().Unix()
	stmt, err := tx.Prepare(fmt.Sprintf(`
UPDATE %spull_request SET pull_request_status = ?, pull_request_timestamp = ? WHERE rowid = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt.Exec(model.PULL_REQUEST_CLOSED_AS_MERGED, t, absId)
	if err != nil { return err }
	stmt2, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %spull_request_event(pull_request_abs_id, event_type, event_timestamp, event_author, event_content)
VALUES (?,?,?,?,?)
`, pfx))
	if err != nil { return err }
	_, err = stmt2.Exec(absId, model.PULL_REQUEST_EVENT_CLOSE_AS_MERGED, t, username, "")
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) CommentOnPullRequest(absId int64, author string, content string) (*model.PullRequestEvent, error) {
	pfx := dbif.config.Database.TablePrefix
	t := time.Now().Unix()
	tx, err := dbif.connection.Begin()
	if err != nil { return nil, err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %spull_request_event(pull_request_abs_id, event_type, event_timestamp, event_author, event_content) VALUES (?,?,?,?,?)
`, pfx))
	if err != nil { return nil, err }
	eventContentString := content
	_, err = stmt.Exec(absId, model.PULL_REQUEST_EVENT_COMMENT, t, author, eventContentString)
	if err != nil { return nil, err }
	err = tx.Commit()
	if err != nil { return nil, err }
	return &model.PullRequestEvent{
		PRAbsId: absId,
		EventType: model.PULL_REQUEST_EVENT_COMMENT,
		EventTimestamp: t,
		EventAuthor: author,
		EventContent: eventContentString,
	}, nil
}

func (dbif *SqliteAegisDatabaseInterface) CommentOnPullRequestCode(absId int64, comment *model.PullRequestCommentOnCode) (*model.PullRequestEvent, error) {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return nil, err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %spull_request_event(pull_request_abs_id, event_type, event_timestamp, event_author, event_content)
VALUES (?,?,?,?,?)
`, pfx))
	if err != nil { return nil, err }
	t := time.Now().Unix()
	contentBytes, _ := json.Marshal(comment)
	contentString := string(contentBytes)
	_, err = stmt.Exec(absId, model.PULL_REQUEST_EVENT_COMMENT_ON_CODE, t, comment.Username, contentString)
	if err != nil { return nil, err }
	err = tx.Commit()
	if err != nil { return nil, err }
	return &model.PullRequestEvent{
		PRAbsId: absId,
		EventType: model.PULL_REQUEST_EVENT_COMMENT_ON_CODE,
		EventTimestamp: t,
		EventAuthor: comment.Username,
		EventContent: contentString,
	}, nil
}

func (dbif *SqliteAegisDatabaseInterface) ClosePullRequestAsNotMerged(absid int64, author string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %spull_request_event(pull_request_abs_id, event_type, event_timestamp, event_author, event_content)
VALUES (?,?,?,?,?)
`, pfx))
	if err != nil { return err }
	t := time.Now().Unix()
	_, err = stmt.Exec(absid, model.PULL_REQUEST_EVENT_CLOSE_AS_NOT_MERGED, t, author, new(string))
	if err != nil { return err }
	stmt2, err := tx.Prepare(fmt.Sprintf(`
UPDATE %spull_request
SET pull_request_status = ?
WHERE rowid = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt2.Exec(model.PULL_REQUEST_CLOSED_AS_NOT_MERGED, absid)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) ReopenPullRequest(absid int64, author string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %spull_request_event(pull_request_abs_id, event_type, event_timestamp, event_author, event_content)
VALUES (?,?,?,?,?)
`, pfx))
	if err != nil { return err }
	t := time.Now().Unix()
	_, err = stmt.Exec(absid, model.PULL_REQUEST_EVENT_REOPEN, t, author, new(string))
	if err != nil { return err }
	stmt2, err := tx.Prepare(fmt.Sprintf(`
UPDATE %spull_request
SET pull_request_status = ?
WHERE rowid = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt2.Exec(model.PULL_REQUEST_OPEN, absid)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteAegisDatabaseInterface) CountPullRequest(query string, namespace string, name string, filterType int) (int, error) {
	pfx := dbif.config.Database.TablePrefix
	statusClause := ""
	switch filterType {
	case 0: statusClause = ""
	case 1: statusClause = "AND pull_request_status = 1"
	case 2: statusClause = "AND NOT (pull_request_status = 1)"
	}
	queryClause := ""
	if query != "" { queryClause = "AND title LIKE ? ESCAPE ?" }
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %spull_request
WHERE receiver_namespace = ? AND receiver_name = ? %s %s
`, pfx, statusClause, queryClause))
	if err != nil { return 0, err }
	var r *sql.Row
	if query == "" {
		r = stmt.QueryRow(namespace, name)
	} else {
		pat := ToSqlSearchPattern(query)
		r = stmt.QueryRow(namespace, name, pat, "\\")
	}
	if r.Err() != nil { return 0, r.Err() }
	var res int
	err = r.Scan(&res)
	if err != nil { return 0, err }
	return res, nil
}

func (dbif *SqliteAegisDatabaseInterface) SearchPullRequestPaginated(query string, namespace string, name string, filterType int, pageNum int, pageSize int) ([]*model.PullRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	statusClause := ""
	switch filterType {
	case 0: statusClause = ""
	case 1: statusClause = "AND pull_request_status = 1"
	case 2: statusClause = "AND NOT (pull_request_status = 1)"
	case 3: statusClause = "AND pull_request_status = 2"
	case 4: statusClause = "AND pull_request_status = 3"
	}
	queryClause := ""
	if query != "" { queryClause = "AND title LIKE ? ESCAPE ?" }
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid, username, pull_request_id, title, receiver_branch, provider_namespace, provider_name, provider_branch, merge_conflict_check_result, merge_conflict_check_timestamp, pull_request_status, pull_request_timestamp
FROM %spull_request
WHERE receiver_namespace = ? AND receiver_name = ? %s %s
ORDER BY pull_request_timestamp DESC LIMIT ? OFFSET ?
`, pfx, statusClause, queryClause))
	if err != nil { return nil, err }
	var r *sql.Rows
	if query == "" {
		r, err = stmt.Query(namespace, name, pageSize, pageNum*pageSize)
	} else {
		pat := ToSqlSearchPattern(query)
		r, err = stmt.Query(namespace, name, pat, "\\", pageSize, pageNum*pageSize)
	}
	if r.Err() != nil { return nil, r.Err() }
	res := make([]*model.PullRequest, 0)
	for r.Next() {
		var prid, absid, prtime int64
		var status int
		var username, title, receiverBranch string
		var providerNamespace, providerName, provideBranch string
		var mergeCheckResultString string
		var mergeCheckTimestamp int64
		err = r.Scan(&absid, &username, &prid, &title, &receiverBranch, &providerNamespace, &providerName, &provideBranch, &mergeCheckResultString, &mergeCheckTimestamp, &status, &prtime)
		if err != nil { return nil, err }
		var mergeCheckResult *gitlib.MergeCheckResult = nil
		if len(mergeCheckResultString) > 0 {		
			err = json.Unmarshal([]byte(mergeCheckResultString), &mergeCheckResult)
			if err != nil { return nil, err }
		}
		res = append(res, &model.PullRequest{
			PRId: prid,
			PRAbsId: absid,
			Title: title,
			Author: username,
			Timestamp: prtime,
			ReceiverNamespace: namespace,
			ReceiverName: name,
			ReceiverBranch: receiverBranch,
			ProviderNamespace: providerNamespace,
			ProviderName: providerName,
			ProviderBranch: provideBranch,
			Status: status,
			MergeCheckResult: mergeCheckResult,
			MergeCheckTimestamp: mergeCheckTimestamp,
		})
	}
	return res, nil
}

