# Auth Service
 
Microsserviço de autenticação da plataforma **ToggleMaster**, responsável pela criação e validação de API Keys.
 
## Visão Geral
 
O Auth Service é o gateway de segurança do ToggleMaster. Ele gerencia o ciclo de vida das chaves de API utilizadas pelos demais microsserviços para autenticação via header `Authorization: Bearer <key>`.
 
## Tecnologias
 
| Componente | Tecnologia |
|---|---|
| Linguagem | Go 1.22+ |
| Banco de Dados | PostgreSQL (RDS) |
| Container | Docker (multi-stage build) |
| Orquestração | Kubernetes (EKS) |
| Registry | Amazon ECR |
| CI/CD | GitHub Actions + ArgoCD (GitOps) |
 
## Endpoints
 
| Método | Rota | Descrição |
|---|---|---|
| `GET` | `/health` | Health check do serviço |
| `POST` | `/keys` | Cria uma nova API Key (requer Master Key) |
| `GET` | `/validate` | Valida uma API Key via header Authorization |
 
## Variáveis de Ambiente
 
| Variável | Descrição |
|---|---|
| `PORT` | Porta do serviço (padrão: `8001`) |
| `DATABASE_URL` | String de conexão PostgreSQL |
| `MASTER_KEY` | Chave mestra para criação de API Keys |
 
## Pipeline CI/CD (DevSecOps)
 
O workflow do GitHub Actions executa os seguintes estágios:
 
1. **Build & Unit Test** — Compilação e execução dos testes unitários
2. **Linter** — Análise estática com `golangci-lint`
3. **Security Scan** — SAST com `gosec` + SCA com `Trivy` (bloqueia vulnerabilidades críticas)
4. **Docker Build & Push** — Build da imagem, scan com Trivy e push para o ECR
5. **GitOps Update** — Atualiza a tag da imagem no repositório `deploy-auth-service`
 
## Deploy (GitOps)
 
O deploy segue o modelo GitOps com ArgoCD. Ao final do pipeline de CI, a tag da imagem é atualizada automaticamente no repositório [`deploy-auth-service`](https://github.com/brianmonteiro54/deploy-auth-service), e o ArgoCD sincroniza a mudança no cluster EKS.
 
## Executando Localmente
 
```bash
# Configurar variáveis
cp .env.example .env
 
# Rodar
go mod download
go run .
```
 
## Estrutura do Projeto
 
```
├── .github/workflows/ci.yaml   # Pipeline CI/CD
├── db/init.sql                  # Script de inicialização do banco
├── Dockerfile                   # Build multi-stage (Go)
├── handlers.go                  # Handlers HTTP
├── handlers_test.go             # Testes unitários
├── key.go                       # Lógica de hash de API Keys
├── main.go                      # Entrypoint da aplicação
├── go.mod / go.sum              # Dependências Go
└── README.md
```

## 📦 Pré-requisitos (Local)

* [Go](https://go.dev/doc/install) (versão 1.21 ou superior)
* [PostgreSQL](https://www.postgresql.org/download/) (rodando localmente ou em um contêiner Docker)

## 🚀 Rodando Localmente

1.  **Clone o repositório** e entre na pasta `auth-service`.

2.  **Prepare o Banco de Dados:**
    * Crie um banco de dados no seu PostgreSQL (ex: `auth_db`).
    * Execute o script `db/init.sql` para criar a tabela `api_keys`:
        ```bash
        psql -U seu_usuario -d auth_db -f db/init.sql
        ```

3.  **Configure as Variáveis de Ambiente:**
    Crie um arquivo chamado `.env` na raiz desta pasta (`auth-service/`) com o seguinte conteúdo:
    ```.env
    # String de conexão do seu banco de dados PostgreSQL
    DATABASE_URL="postgres://SEU_USUARIO:SUA_SENHA@localhost:5432/auth_db"
    
    # Porta que o serviço irá rodar
    PORT="8001"
    
    # Chave mestra para criar novas chaves de API
    MASTER_KEY="admin-secreto-123"
    ```

4.  **Instale as Dependências:**
    ```bash
    go mod tidy
    ```

5.  **Inicie o Serviço:**
    ```bash
    go run .
    ```
    O servidor estará rodando em `http://localhost:8001`.

## 🧪 Testando os Endpoints

Você pode usar `curl` ou Postman.

**1. Verifique a Saúde (Health Check):**
```bash
curl http://localhost:8001/health
```

Saída esperada: `{"status":"ok"}`

**2. Crie uma nova Chave de API (requer a MASTER_KEY):**

```bash
curl -X POST http://localhost:8001/admin/keys \
-H "Content-Type: application/json" \
-H "Authorization: Bearer admin-secreto-123" \
-d '{"name": "meu-primeiro-servico"}'
``` 

Saída esperada (A SUA CHAVE SERÁ DIFERENTE):

```json
{
  "name": "meu-primeiro-servico",
  "key": "tm_key_a1b2c3d4...",
  "message": "Guarde esta chave com segurança! Você não poderá vê-la novamente."
}
```

**3. Valide a Chave que você acabou de criar: (Substitua tm_key_... pela chave exata que você recebeu no passo anterior)**

```bash
curl http://localhost:8001/validate \
-H "Authorization: Bearer tm_key_a1b2c3d4..."
```

Saída esperada: `{"message":"Chave válida"}`

**4. Teste uma Chave Inválida:**

```bash
curl http://localhost:8001/validate \
-H "Authorization: Bearer chave-errada-123"
```

Saída esperada: `Chave de API inválida ou inativa`
