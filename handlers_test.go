package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func newTestApp(t *testing.T) (*App, sqlmock.Sqlmock, func()) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("erro ao criar sqlmock: %v", err)
	}

	app := &App{
		DB:        db,
		MasterKey: "admin-secreto-123",
	}

	cleanup := func() {
		if err := db.Close(); err != nil {
    t.Errorf("error closing database: %v", err)
}
	}

	return app, mock, cleanup
}

func TestHealthHandler(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	app.healthHandler(w, req)

	resp := w.Result()
	defer func() {
    if err := resp.Body.Close(); err != nil {
        t.Errorf("error closing response body: %v", err)
    }
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status esperado %d, obtido %d", http.StatusOK, resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("erro ao decodificar resposta: %v", err)
	}

	if body["status"] != "ok" {
		t.Fatalf("status esperado 'ok', obtido '%s'", body["status"])
	}
}

func TestMasterKeyAuthMiddleware_Authorized(t *testing.T) {
	app := &App{MasterKey: "admin-secreto-123"}

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := app.masterKeyAuthMiddleware(next)

	req := httptest.NewRequest(http.MethodPost, "/admin/keys", nil)
	req.Header.Set("Authorization", "Bearer admin-secreto-123")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if !called {
		t.Fatal("handler principal não foi chamado")
	}

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status esperado %d, obtido %d", http.StatusOK, w.Result().StatusCode)
	}
}

func TestMasterKeyAuthMiddleware_Forbidden(t *testing.T) {
	app := &App{MasterKey: "admin-secreto-123"}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler principal não deveria ser chamado")
	})

	handler := app.masterKeyAuthMiddleware(next)

	req := httptest.NewRequest(http.MethodPost, "/admin/keys", nil)
	req.Header.Set("Authorization", "Bearer chave-errada")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusForbidden {
		t.Fatalf("status esperado %d, obtido %d", http.StatusForbidden, w.Result().StatusCode)
	}
}

func TestValidateKeyHandler_MissingAuthorization(t *testing.T) {
	app, _, cleanup := newTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/validate", nil)
	w := httptest.NewRecorder()

	app.validateKeyHandler(w, req)

	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Fatalf("status esperado %d, obtido %d", http.StatusUnauthorized, w.Result().StatusCode)
	}
}

func TestValidateKeyHandler_InvalidKey(t *testing.T) {
	app, mock, cleanup := newTestApp(t)
	defer cleanup()

	key := "tm_key_invalida"
	keyHash := hashAPIKey(key)

	mock.ExpectQuery(`SELECT id FROM api_keys WHERE key_hash = \$1 AND is_active = true`).
		WithArgs(keyHash).
		WillReturnError(sql.ErrNoRows)

	req := httptest.NewRequest(http.MethodGet, "/validate", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	w := httptest.NewRecorder()

	app.validateKeyHandler(w, req)

	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Fatalf("status esperado %d, obtido %d", http.StatusUnauthorized, w.Result().StatusCode)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectativas do mock não atendidas: %v", err)
	}
}

func TestValidateKeyHandler_ValidKey(t *testing.T) {
	app, mock, cleanup := newTestApp(t)
	defer cleanup()

	key := "tm_key_valida"
	keyHash := hashAPIKey(key)

	rows := sqlmock.NewRows([]string{"id"}).AddRow(1)

	mock.ExpectQuery(`SELECT id FROM api_keys WHERE key_hash = \$1 AND is_active = true`).
		WithArgs(keyHash).
		WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/validate", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	w := httptest.NewRecorder()

	app.validateKeyHandler(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status esperado %d, obtido %d", http.StatusOK, w.Result().StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Result().Body).Decode(&body); err != nil {
		t.Fatalf("erro ao decodificar resposta: %v", err)
	}

	if body["message"] != "Chave válida" {
		t.Fatalf("mensagem inesperada: %s", body["message"])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectativas do mock não atendidas: %v", err)
	}
}

func TestCreateKeyHandler_MethodNotAllowed(t *testing.T) {
	app, _, cleanup := newTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/admin/keys", nil)
	w := httptest.NewRecorder()

	app.createKeyHandler(w, req)

	if w.Result().StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status esperado %d, obtido %d", http.StatusMethodNotAllowed, w.Result().StatusCode)
	}
}

func TestCreateKeyHandler_InvalidJSON(t *testing.T) {
	app, _, cleanup := newTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/admin/keys", strings.NewReader(`{invalido`))
	w := httptest.NewRecorder()

	app.createKeyHandler(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("status esperado %d, obtido %d", http.StatusBadRequest, w.Result().StatusCode)
	}
}

func TestCreateKeyHandler_MissingName(t *testing.T) {
	app, _, cleanup := newTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/admin/keys", strings.NewReader(`{"name":""}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	app.createKeyHandler(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("status esperado %d, obtido %d", http.StatusBadRequest, w.Result().StatusCode)
	}
}

func TestCreateKeyHandler_Success(t *testing.T) {
	app, mock, cleanup := newTestApp(t)
	defer cleanup()

	body := `{"name":"meu-servico"}`

	mock.ExpectQuery(`INSERT INTO api_keys \(name, key_hash\) VALUES \(\$1, \$2\) RETURNING id`).
		WithArgs("meu-servico", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	req := httptest.NewRequest(http.MethodPost, "/admin/keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	app.createKeyHandler(w, req)

	if w.Result().StatusCode != http.StatusCreated {
		t.Fatalf("status esperado %d, obtido %d", http.StatusCreated, w.Result().StatusCode)
	}

	var resp CreateKeyResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&resp); err != nil {
		t.Fatalf("erro ao decodificar resposta: %v", err)
	}

	if resp.Name != "meu-servico" {
		t.Fatalf("name esperado 'meu-servico', obtido '%s'", resp.Name)
	}

	if !strings.HasPrefix(resp.Key, "tm_key_") {
		t.Fatalf("chave gerada inválida: %s", resp.Key)
	}

	if resp.Message == "" {
		t.Fatal("mensagem não deveria estar vazia")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectativas do mock não atendidas: %v", err)
	}
}

func TestCreateKeyHandler_DBError(t *testing.T) {
	app, mock, cleanup := newTestApp(t)
	defer cleanup()

	body := `{"name":"meu-servico"}`

	mock.ExpectQuery(`INSERT INTO api_keys \(name, key_hash\) VALUES \(\$1, \$2\) RETURNING id`).
		WithArgs("meu-servico", sqlmock.AnyArg()).
		WillReturnError(sql.ErrConnDone)

	req := httptest.NewRequest(http.MethodPost, "/admin/keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	app.createKeyHandler(w, req)

	if w.Result().StatusCode != http.StatusInternalServerError {
		t.Fatalf("status esperado %d, obtido %d", http.StatusInternalServerError, w.Result().StatusCode)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectativas do mock não atendidas: %v", err)
	}
}

func TestHashAPIKey(t *testing.T) {
	key := "minha-chave"
	hash1 := hashAPIKey(key)
	hash2 := hashAPIKey(key)

	if hash1 != hash2 {
		t.Fatal("o hash deveria ser determinístico para a mesma chave")
	}

	if len(hash1) != 64 {
		t.Fatalf("hash SHA-256 em hex deve ter 64 caracteres, obtido %d", len(hash1))
	}
}

func TestGenerateAPIKey(t *testing.T) {
	key, err := generateAPIKey()
	if err != nil {
		t.Fatalf("não esperava erro ao gerar chave: %v", err)
	}

	if !strings.HasPrefix(key, "tm_key_") {
		t.Fatalf("prefixo inválido: %s", key)
	}

	if len(key) <= len("tm_key_") {
		t.Fatalf("chave gerada muito curta: %s", key)
	}
}

func TestValidateKeyHandler_DBGenericError(t *testing.T) {
	app, mock, cleanup := newTestApp(t)
	defer cleanup()

	key := "tm_key_qualquer"
	keyHash := hashAPIKey(key)

	mock.ExpectQuery(`SELECT id FROM api_keys WHERE key_hash = \$1 AND is_active = true`).
		WithArgs(keyHash).
		WillReturnError(sql.ErrConnDone)

	req := httptest.NewRequest(http.MethodGet, "/validate", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	w := httptest.NewRecorder()

	app.validateKeyHandler(w, req)

	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Fatalf("status esperado %d, obtido %d", http.StatusUnauthorized, w.Result().StatusCode)
	}
}

func TestMasterKeyAuthMiddleware_MissingHeader(t *testing.T) {
	app := &App{MasterKey: "admin-secreto-123"}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler principal não deveria ser chamado")
	})

	handler := app.masterKeyAuthMiddleware(next)

	req := httptest.NewRequest(http.MethodPost, "/admin/keys", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusForbidden {
		t.Fatalf("status esperado %d, obtido %d", http.StatusForbidden, w.Result().StatusCode)
	}
}