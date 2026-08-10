# OLED Display para Orange Pi

Aplicação em Go que exibe métricas do sistema (CPU, RAM, IP e hora) em uma tela OLED SSD1306 via I2C, rodando em um container Docker.

## Arquitetura

- **Host de build**: sua máquina (PC) — onde o código é compilado e a imagem é enviada para um registry.
- **Orange Pi PC Plus**: processador **ARM v7 (32 bits)** — puxa a imagem do registry e roda o container com acesso ao I2C.

> ⚠️ **Importante**: a imagem precisa ser buildada para `linux/arm/v7`, senão o Orange Pi não consegue executar o binário (erro `exec format error`).

---

## Pré-requisitos

### Na sua máquina (host de build)
- Docker com **Buildx** (incluído no Docker 20.10+)
- Acesso ao projeto
- Registry da sua máquina configurado como **inseguro** (HTTP) — tanto no daemon do Docker quanto no BuildKit

### No Orange Pi
- Docker instalado
- Registry da sua máquina configurado como **inseguro** (HTTP)

---

## 1. Subir o registry temporário (na sua máquina)

O registry é **temporário** (`--rm`, sem volume) — ele some quando o PC desliga. Toda vez que ligar o PC, suba de novo:

```bash
docker run -d --rm --name registry -p 5000:5000 registry:2
```

> Substitua `192.168.1.3` pelo IP da sua máquina em todos os comandos abaixo.

---

## 2. Configurar o registry inseguro (na sua máquina)

Como o registry é acessado via HTTP (não HTTPS), o seu PC precisa aceitá-lo em **dois** lugares: no daemon do Docker e no BuildKit.

### 2.1 Daemon do Docker

```bash
sudo nano /etc/docker/daemon.json
```

```json
{
  "insecure-registries": ["192.168.1.3:5000"]
}
```

Reinicie o Docker:

```bash
sudo systemctl restart docker
```

### 2.2 BuildKit (necessário para o `--push` com buildx)

O BuildKit roda em um container separado e **não herda** a config do daemon. Crie o arquivo:

```bash
sudo mkdir -p /etc/buildkit
sudo nano /etc/buildkit/buildkitd.toml
```

```toml
[registry."192.168.1.3:5000"]
  http = true
  insecure = true
```

---

## 3. Buildar a imagem para ARM v7

### 3.1 Habilitar suporte a multi-arquitetura (uma vez só)

```bash
docker run --privileged --rm tonistiigi/binfmt --install all
```

### 3.2 Criar um builder multi-arquitetura (uma vez só)

```bash
docker buildx create --name multiarch --use
```

### 3.3 Buildar e enviar para o registry

```bash
cd ~/projetos/pessoal/oled
docker buildx build --platform linux/arm/v7 -t 192.168.1.3:5000/oled:latest --push .
```

O `--push` envia a imagem **ARM v7** diretamente para o registry.

> ⚠️ **Não use `--load` + `docker push`**: o `--load` só carrega a plataforma nativa (amd64) no daemon, então o `docker push` seguinte enviaria a imagem errada (amd64), causando `exec format error` no Orange Pi.

### Opcional: imagem multi-arquitetura

Para gerar uma imagem que funcione em qualquer arquitetura (amd64, arm/v7, arm64):

```bash
docker buildx build --platform linux/amd64,linux/arm/v7,linux/arm64 -t 192.168.1.3:5000/oled:latest --push .
```

---

## 4. Configurar o registry inseguro (no Orange Pi)

Como o registry é acessado via HTTP (não HTTPS), o Docker do Orange Pi precisa aceitá-lo:

```bash
sudo nano /etc/docker/daemon.json
```

```json
{
  "insecure-registries": ["192.168.1.3:5000"]
}
```

Reinicie o Docker:

```bash
sudo systemctl restart docker
```

---

## 5. Rodar no Orange Pi

```bash
cd ~/oled
docker compose up -d
```

Ver os logs:

```bash
docker compose logs -f
```

---

## Deploy do código para o Orange Pi

Para copiar o código deste repositório para o Orange Pi:

```bash
./deploy.sh
```

Isso copia todos os arquivos (exceto `.git` e `deploy.sh`) para `/home/homelab/oled` via `rsync`.

---

## Estrutura do projeto

```
oled/
├── main.go            # Loop principal: lê métricas e desenha na tela
├── network.go         # Obtém IP e SSID da rede
├── stats.go           # Lê CPU e RAM de /proc
├── ssd1306/           # Driver da tela OLED SSD1306
├── Dockerfile         # Build multi-estágio
├── docker-compose.yml # Configuração do container (I2C + rede host)
└── deploy.sh          # Copia o código para o Orange Pi
```

---

## Botão físico (liga/desliga o display)

O display desliga sozinho após **1 minuto** ligado para economizar energia. Para acendê-lo, pressione um **push button** conectado a um GPIO do Orange Pi (botão puxando o pino para **GND**).

### Configurando

No código (`main.go`), o GPIO já está configurado para **PA7** (offset 7 do chip):

```go
const gpioBotao = 7 // PA7 -> pino físico 29 do header
```

O número é o **offset da linha dentro do `gpiochip0`**. No Allwinner H3, o offset é calculado pela fórmula:

```
offset = (letra do banco - 1) * 32 + pino
```

Exemplos para o Orange Pi PC Plus (H3):
- **PA7** → pino físico **29** → `(1-1)*32 + 7 = 7`  ✅ (recomendado)
- **PA8** → pino físico **31** → `(1-1)*32 + 8 = 8`
- **PC4** → pino físico **16** → `(3-1)*32 + 4 = 68`

### Conexão

Ligação **simples** (apenas 2 fios, **sem resistor**):

```
 pino 29 (PA7) ──┐
                 ├──── push button ──── pino 30 (GND)
```

- **Push button** entre o **pino físico 29 (PA7)** e **GND (pino 30)**.
- O display usa a interface de **dispositivo de caracteres** (`/dev/gpiochip0`) via **libgpiod**, que **habilita o pull-up interno por software**. Por isso **não precisa de resistor externo** — o pino já é puxado para HIGH quando o botão está solto, e cai para LOW ao pressionar.

### Como funciona no container

O app usa `/dev/gpiochip0` (device de caracteres), acessível pois o container roda `privileged` / com o device montado:

```yaml
volumes:
  - /dev/gpiochip0:/dev/gpiochip0:rw
```

O código (`gpio.go`) solicita a linha 7 do chip como **entrada com pull-up** (`gpiocdev.AsInput` + `gpiocdev.WithPullUp`) e faz **polling** a cada 10 ms com debounce por estabilidade. A cada pressionada, o display liga e o timer de 1 minuto é reiniciado.

> ⚠️ **Permissão**: acessar `/dev/gpiochip0` exige **root**. No container o processo roda como root, então funciona. Se testar fora do container (`go run .`), rode com `sudo`.

---

## Solução de problemas

| Erro | Causa | Solução |
|------|-------|---------|
| `server gave HTTP response to HTTPS client` (no Orange Pi) | Registry não marcado como inseguro no Orange Pi | Configurar `insecure-registries` no daemon do Orange Pi |
| `server gave HTTP response to HTTPS client` (no push/buildx) | BuildKit não aceita o registry inseguro | Configurar `/etc/buildkit/buildkitd.toml` com `http = true` |
| `not found` ao puxar | Imagem não enviada ao registry | Rodar `docker buildx build --push` na sua máquina |
| `exec format error` | Imagem buildada para arquitetura errada (amd64 em vez de arm/v7) | Buildar com `--platform linux/arm/v7` e `--push` (não usar `--load` + `docker push`) |
| `connection refused` | Registry não está rodando | Subir o registry na sua máquina |
