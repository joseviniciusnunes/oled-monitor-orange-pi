# ---- Estágio de build ----
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copia os módulos e baixa as dependências (aproveita cache)
COPY go.mod go.sum* ./
RUN go mod download

# Copia o código-fonte
COPY . .

# Compila o binário estático
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /oled .

# ---- Estágio final (runtime) ----
FROM alpine:3.20

# i2c-tools é opcional, mas útil para depuração dentro do container
RUN apk add --no-cache i2c-tools

WORKDIR /app

# Copia o binário compilado
COPY --from=builder /oled .

# O container precisa de acesso ao dispositivo I2C do host.
# O dispositivo deve ser passado via --device /dev/i2c-1 (ou docker-compose).
CMD ["./oled"]