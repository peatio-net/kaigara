package main

import (
	"os"
	"testing"

	"github.com/openware/kaigara/cmd/testenv"
	"github.com/openware/kaigara/pkg/config"
	"github.com/openware/kaigara/pkg/sql"
	"github.com/openware/pkg/database"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

var vars = []string{
	"FINEX_DATABASE_USERNAME",
	"FINEX_DATABASE_PASSWORD",
	"FINEX_DATABASE_NAME",
	"FINEX_DATABASE_HOST",
	"FINEX_INFLUX_USERNAME",
	"FINEX_INFLUX_PASSWORD",
	"FINEX_INFLUX_HOST",
	"GOTRUE_DATABASE_USERNAME",
	"GOTRUE_DATABASE_PASSWORD",
	"GOTRUE_DATABASE_NAME",
	"GOTRUE_DATABASE_HOST",
	"PGRST_DB_USERNAME",
	"PGRST_DB_PASS",
	"PGRST_DB_NAME",
	"PGRST_DB_HOST",
	"REALTIME_DB_USERNAME",
	"REALTIME_DB_PASS",
	"REALTIME_DB_NAME",
	"REALTIME_DB_HOST",
}

func TestMain(m *testing.M) {
	var err error
	if conf, err = config.NewKaigaraConfig(); err != nil {
		panic(err)
	}
	ls = nil

	// exec test and this returns an exit code to pass to os
	code := m.Run()

	os.Exit(code)
}

func TestAppNamesToLoggingName(t *testing.T) {
	conf.AppNames = "peatio,peatio_daemons"
	assert.Equal(t, "peatio&peatio_daemons", appNamesToLoggingName())

	conf.AppNames = "peatio"
	assert.Equal(t, "peatio", appNamesToLoggingName())
	assert.NotEqual(t, "peatio&", appNamesToLoggingName())
	assert.NotEqual(t, "&peatio", appNamesToLoggingName())
}

func TestKaigaraPrintenvVault(t *testing.T) {
	conf.Storage = "vault"
	conf.AppNames = "finex,frontdex,gotrue,postgrest,realtime,storage"
	ss := testenv.GetStorage(conf)

	for _, v := range vars {
		kaigaraRun(ss, "printenv", []string{v})
	}
}

func TestKaigaraPrintenvSql(t *testing.T) {
	conf.Storage = "sql"
	conf.AppNames = "finex,frontdex,gotrue,postgrest,realtime,storage"
	ss := testenv.GetStorage(conf)

	for _, v := range vars {
		kaigaraRun(ss, "printenv", []string{v})
	}

	// Cleanup data
	sqlDB, err := database.Connect(&conf.DBConfig)
	if err != nil {
		t.Fatal(err)
	}

	tx := sqlDB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&sql.Model{})
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
}
