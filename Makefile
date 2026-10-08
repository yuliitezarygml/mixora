.PHONY: dev infra recommendations embeddings down logs test test-client test-backend mobile-dev mobile-test mobile-apk mobile-ios-simulator

dev:
	docker compose up --build

infra:
	docker compose up -d postgres redis mailpit

recommendations:
	MIXORA_GORSE_URL=http://gorse:8088 MIXORA_EMBEDDINGS_URL= docker compose --profile recommendations up -d --build postgres redis mailpit gorse api

embeddings:
	MIXORA_GORSE_URL=http://gorse:8088 MIXORA_EMBEDDINGS_URL=http://ollama:11434 MIXORA_EMBEDDINGS_REQUIRED=true docker compose --profile recommendations --profile embeddings up -d --build postgres redis mailpit gorse ollama ollama-pull api

down:
	docker compose down

logs:
	docker compose logs -f api

test: test-backend test-client

test-backend:
	cd beckend && go test ./...

test-client:
	cd mixora-client && npm test

mobile-dev:
	cd app-andorid-ios && npm run native:dev

mobile-test:
	cd app-andorid-ios && npm test
	cd app-andorid-ios/src-tauri && cargo test --locked --lib

mobile-apk:
	cd app-andorid-ios && npm run android:apk

mobile-ios-simulator:
	cd app-andorid-ios && npm run ios:simulator
