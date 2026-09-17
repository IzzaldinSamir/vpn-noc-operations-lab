.PHONY: init rotate-keys up down reset ps logs health smoke validate recover drill-peer-down drill-service-down drill-dns-failure drill-latency

init:
	go run ./cmd/keygen
	@test -f .env || cp .env.example .env

rotate-keys:
	go run ./cmd/keygen --force

up:
	@test -f runtime/wireguard/site-a/wg0.conf || $(MAKE) init
	docker compose up -d --build --wait

down:
	docker compose down

reset:
	docker compose down --volumes --remove-orphans

ps:
	docker compose ps

logs:
	docker compose logs --follow --tail=100

health:
	./scripts/health-check.sh

smoke:
	./scripts/smoke-test.sh

validate:
	./scripts/validate.sh

recover:
	./scripts/drill.sh recover

drill-peer-down:
	./scripts/drill.sh peer-down

drill-service-down:
	./scripts/drill.sh service-down

drill-dns-failure:
	./scripts/drill.sh dns-failure

drill-latency:
	./scripts/drill.sh high-latency
