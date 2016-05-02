//
// copywrong you're mom 2015
//

// package uguu implements uguu-tracker storage driver using postgres
package uguu

import (
	"crypto/rand"

	"database/sql"
	_ "github.com/lib/pq"

	"encoding/base32"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/golang/glog"

	"github.com/majestrate/chihaya/backend"
	"github.com/majestrate/chihaya/config"
	"github.com/majestrate/chihaya/tracker/models"
)

// driver for uguu-tracker
type uguuDriver struct{}

type UguuSQL struct {
	// database connection
	conn *sql.DB
}

var cfg_version = "uguu.version"

// what database version are we at
func (u *UguuSQL) Version() (version string, err error) {
	err = u.conn.QueryRow("SELECT val FROM config WHERE key = $1", cfg_version).Scan(&version)
	return
}

func (u *UguuSQL) setVersion(version string) (err error) {
	_, err = u.conn.Exec("DELETE FROM config WHERE key = $1", cfg_version)
	if err == nil {
		_, err = u.conn.Exec("INSERT INTO config(key, val) VALUES($1, $2)", cfg_version, version)
	}
	return
}

// create initial version 0 tables
func (u *UguuSQL) InitTables() (err error) {
	_, err = u.conn.Exec("CREATE TABLE IF NOT EXISTS config(key VARCHAR(255) PRIMARY KEY, val VARCHAR(255) NOT NULL)")
	if err == nil {
		var version string
		version, err = u.Version()
		if len(version) == 0 {
			err = u.setVersion("0")
		}
	}
	return
}

// return true if the version string is the latest version
func (u *UguuSQL) LatestVersion(version string) (latest bool) {
	latest = version == "1"
	return
}

// upgrade to the next database version given the current version
func (u *UguuSQL) UpgradeToNext(version string) (err error) {
	glog.Errorf("upgrade database at version %s to next version", version)

	pre_queries := []string{}
	table_defs := make(map[string]string)
	table_order := []string{}
	post_queries := []string{}
	next_version := ""

	if version == "0" {
		// migrate to version 1
		next_version = "1"
		table_defs["torrents"] = `(
                                torrent_id BIGSERIAL PRIMARY KEY,
                                torrent_upload_user_id BIGINT NOT NULL,
                                torrent_infohash VARCHAR(40) NOT NULL,
                                torrent_last_active BIGINT NOT NULL DEFAULT 0,
                                torrent_first_active BIGINT NOT NULL DEFAULT 0,
                                torrent_name TEXT NOT NULL,
                                torrent_cat_id INTEGER NOT NULL,
                                torrent_description TEXT NOT NULL,
                                torrent_file_filepath VARCHAR(255) NOT NULL,
                                torrent_uploaded_time BIGINT NOT NULL,
 
                                FOREIGN KEY (torrent_upload_user_id) REFERENCES torrent_users(user_id) ON DELETE CASCADE,
                                FOREIGN KEY (torrent_cat_id) REFERENCES torrent_categories(cat_id) ON DELETE CASCADE
                              )`

		table_defs["torrent_files"] = `(
                                     file_name TEXT NOT NULL,
                                     file_torrent_id BIGINT NOT NULL,
                                     PRIMARY KEY (file_name, file_torrent_id),
                                     FOREIGN KEY (file_torrent_id) REFERENCES torrents(torrent_id) ON DELETE CASCADE
                                   )`

		table_defs["torrent_tags"] = `(
                                    tag_name VARCHAR(255),
                                    tag_torrent_id BIGINT,
                                    PRIMARY KEY (tag_name, tag_torrent_id),
                                    FOREIGN KEY (tag_torrent_id) REFERENCES torrents(torrent_id) ON DELETE CASCADE
                                  )`

		table_defs["torrent_users"] = `(
                                     user_id BIGSERIAL PRIMARY KEY,
                                     user_passkey VARCHAR(255) NOT NULL,
                                     user_login_name VARCHAR(255) NOT NULL,
                                     user_login_cred VARCHAR(255) NOT NULL
                                   )`

		table_defs["torrent_categories"] = `(
                                          cat_id SERIAL PRIMARY KEY,
                                          cat_name VARCHAR(255) NOT NULL,
                                          cat_desc TEXT NOT NULL
                                        )`

		table_order = append(table_order, "torrent_categories")
		table_order = append(table_order, "torrent_users")
		table_order = append(table_order, "torrents")
		table_order = append(table_order, "torrent_tags")
		table_order = append(table_order, "torrent_files")
	} else {
		// invalid version
		return errors.New("invalid version")
	}

	// run pre-conditions
	glog.Infof("run %d preconditions", len(pre_queries))
	for _, q := range pre_queries {
		glog.V(1).Infof(">> %s", q)
		_, err = u.conn.Exec(q)
		if err != nil {
			return
		}
	}

	// create new tables
	glog.Infof("create %d tables", len(table_order))
	for _, t := range table_order {
		glog.Infof("create table %s", t)
		q := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s%s", t, table_defs[t])
		glog.Infof(">> %s", q)
		_, err = u.conn.Exec(q)
		if err != nil {
			return
		}
	}

	// run post-conditions
	glog.Infof("run %d postconditions", len(post_queries))
	for _, q := range pre_queries {
		glog.V(1).Infof(">> %s", q)
		_, err = u.conn.Exec(q)
		if err != nil {
			return
		}
	}
	err = u.setVersion(next_version)
	return
}

// run all migrations
func (u *UguuSQL) Migrate() (err error) {
	var version string
	// ensure initail tables
	err = u.InitTables()
	version, err = u.Version()
	// do migrations
	for err == nil && !u.LatestVersion(version) {
		if err == nil {
			err = u.UpgradeToNext(version)
		}
		version, err = u.Version()
	}
	return
}

// close connection to database
func (u *UguuSQL) Close() (err error) {
	err = u.conn.Close()
	return
}

// ping backend
func (u *UguuSQL) Ping() (err error) {
	err = u.conn.Ping()
	return
}

// record that a bittorrent announce happened
func (u *UguuSQL) RecordAnnounce(delta *models.AnnounceDelta) (err error) {
	// TODO: record ratio
	return
}

// add a torrent to the database
func (u *UguuSQL) AddTorrent(torrent *models.Torrent) (err error) {
	info := torrent.Info
	if info == nil {
		// no torrent info in model
		err = errors.New("torrent has no info")
		glog.Errorf("error while addding torrent: %s", err.Error())
		return
	}
	var hasUser, canUpload bool
	if info.UserID == 0 {
		// no user specified
		// this is an anonymously added torrent
		// TODO: check if we allow it explicitly
		hasUser = true
	} else {
		var count int64
		// do we have this user?
		err = u.conn.QueryRow("SELECT COUNT(*) FROM torrent_users WHERE user_id = $1", info.UserID).Scan(&count)
		if err == nil {
			// set if we have it or not
			hasUser = count > 0
			// TODO: check if they can upload or not
			canUpload = hasUser
		}
	}

	// do we have a user?
	if !hasUser {
		// we don't have this user
		err = models.ErrUserDNE
		return
	}

	// can we upload?
	if !canUpload {
		// nah
		err = errors.New("this user is not allowed to upload")
		return
	}

	var cat_id int64
	err = u.conn.QueryRow(`SELECT cat_id FROM torrent_categories WHERE cat_name = $1 LIMIT 1`, info.Category).Scan(&cat_id)

	if err != nil {
		// no category?
		glog.Errorf("failed to get cat_id: %s", err.Error())
		return
	}

	now := time.Now().UTC().UnixNano()

	var torrent_id int64

	var tx *sql.Tx

	tx, err = u.conn.Begin()
	if err != nil {
		return
	}
	// insert into torrents table
	err = tx.QueryRow(`INSERT INTO torrents
                     (
                       torrent_upload_user_id, 
                       torrent_infohash, 
                       torrent_name, 
                       torrent_cat_id, 
                       torrent_description, 
                       torrent_file_filepath,
                       torrent_uploaded_time
                     )
                     VALUES
                     ( 
                       $1,
                       $2,
                       $3,
                       $4,
                       $5,
                       $6,
                       $7
                     )
                     RETURNING torrent_id`,
		info.UserID,
		torrent.Infohash,
		info.TorrentName,
		cat_id,
		info.Description,
		fmt.Sprintf("%d.torrent", now),
		now).Scan(&torrent_id)

	if err == nil {
		// we inserted it
		if torrent_id > 0 {
			// it's inserted for sure, probably
			// insert tags
			for _, tag := range info.Tags {
				_, err = tx.Exec(`INSERT INTO torrent_tags(tag_name, tag_torrent_id) VALUES($1, $2)`, tag, torrent_id)
				if err != nil {
					glog.Error("failed to insert torrent tag", err.Error())
					err2 := tx.Rollback()
					if err2 != nil {
						glog.Error("failed to rollback transaction", err2.Error())
					}
					return errors.New("database error")
				}
			}
			// insert file records
			for _, file := range info.Files {
				_, err = tx.Exec(`INSERT INTO torrent_files(file_name, file_torrent_id) VALUES($1, $2)`, file, torrent_id)
				if err != nil {
					glog.Error("failed to insert torrent file records", err.Error())
					err2 := tx.Rollback()
					if err2 != nil {
						glog.Error("failed to rollback transaction", err2.Error())
					}
					return errors.New("database error")
				}
			}
			// it gud, let's commit
			err = tx.Commit()
		} else {
			glog.Error("error while addding torrent, inserted row id <= 0")
		}
	}
	if err != nil {
		glog.Errorf("error while addding torrent: %s", err.Error())
	}
	return
}

// generate a passkey
func genPassKey() string {
	var buff [30]byte
	_, _ = io.ReadFull(rand.Reader, buff[:])
	return strings.ToLower(base32.StdEncoding.EncodeToString(buff[:]))
}

// generate a new passkey that doesn't exist in the database already
func (u *UguuSQL) GeneratePasskey() (key string) {
	var count int64
	var err error
	count = -1
	for err == nil {
		tkey := genPassKey()
		err = u.conn.QueryRow(`SELECT COUNT(*) FROM torrent_users WHERE user_passkey = $1`, tkey).Scan(&count)
		if err == nil {
			if count == 0 {
				key = tkey
				break
			}
		} else {
			glog.Errorf("failed to generate passkey: %s", err.Error())
		}
	}
	return
}

// add a user to the database
func (u *UguuSQL) AddUser(user *models.User) (err error) {
	passkey := u.GeneratePasskey()
	if len(passkey) > 0 {
		_, err = u.conn.Exec(`INSERT INTO torrent_users(user_passkey, user_login_name, user_login_cred) VALUES($1, $2, $3)`, passkey, user.Username, user.Cred)
	} else {
		err = errors.New("cannot generate passkey")
	}
	return
}

// delete an already existing torrent
func (u *UguuSQL) DeleteTorrent(torrent *models.Torrent) (err error) {
	_, err = u.conn.Exec(`DELETE FROM torrents WHERE torrent_infohash = $1`, torrent.Infohash)
	return
}

func (u *UguuSQL) DeleteUser(user *models.User) (err error) {
	_, err = u.conn.Exec(`DELETE FROM torrent_users WHERE user_passkey = $1`, user.Passkey)
	return
}

func (u *UguuSQL) GetTorrentByInfoHash(infohash string) (t *models.Torrent, err error) {
	var count int64
	err = u.conn.QueryRow(`SELECT COUNT(*) FROM torrents WHERE torrent_infohash = $1`, infohash).Scan(&count)
	if err == nil {
		if count > 0 {
			t = new(models.Torrent)
			t.Infohash = infohash
		} else {
			err = models.ErrTorrentDNE
		}
	}
	return
}

func (u *UguuSQL) GetUserByPassKey(passkey string) (user *models.User, err error) {
	obtained := new(models.User)
	err = u.conn.QueryRow(`SELECT user_id, user_passkey, user_login_name, user_login_cred FROM torrent_users WHERE user_passkey = $1 LIMIT 1`, passkey).Scan(&obtained.ID, &obtained.Passkey, &obtained.Username, &obtained.Cred)
	if err == nil {
		user = obtained
	}
	return
}

func (u *UguuSQL) GetCategories() (cats []*models.TorrentCategory, err error) {
	return
}

func (u *UguuSQL) LoadTorrents(ids []uint64) (torrents []*models.Torrent, err error) {
	err = errors.New("uguu load torrents not implemented")
	return
}

// load users given an array of ids
func (u *UguuSQL) LoadUsers(ids []uint64) (users []*models.User, err error) {
	for _, id := range ids {
		user := new(models.User)
		err = u.conn.QueryRow(`SELECT user_id, user_passkey, user_login_name, user_login_cred FROM torrent_users WHERE user_id = $1 LIMIT 1`, id).Scan(&user.ID, &user.Passkey, &user.Username, &user.Cred)
		if err != nil {
			return
		}
		users = append(users, user)
	}
	return
}

// extract database login creds from map
func extractDBCreds(param map[string]string) (str string, err error) {
	var ok bool
	str, ok = param["url"]
	if !ok {
		err = errors.New("no url parameter")
	}
	return
}

// create a new uguu driver
func (d *uguuDriver) New(cfg *config.DriverConfig) (c backend.Conn, err error) {
	var url string
	// get db creds
	url, err = extractDBCreds(cfg.Params)
	if err == nil {
		// we got them db creds now create a connection
		uguu := new(UguuSQL)
		uguu.conn, err = sql.Open("postgres", url)
		if err == nil {
			// do all migrations
			err = uguu.Migrate()
			if err == nil {
				// migration gud
				// hustan we are go for launch
				c = uguu
			} else {
				// migration failed
				// close the database connection
				uguu.Close()
				glog.Error("migration failed", err)
			}
		}
	}
	return
}

func init() {
	backend.Register("uguu", &uguuDriver{})
}
