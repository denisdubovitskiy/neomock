.PHONY: docs-install docs-serve docs-build act act-list lint

# Документация: локальное окружение poetry (pyproject.toml, .venv в репозитории).

docs-install:
	poetry install

docs-serve: docs-install
	poetry run mkdocs serve

docs-build: docs-install
	poetry run mkdocs build --strict

# Линтер: golangci-lint в Docker (Docker Desktop/Lima — через docker context).
# Кэш анализа — в именованном volume, повторные прогоны быстрые.

LINT_IMAGE ?= golangci/golangci-lint:v2.11.4

lint:
	DOCKER_HOST=$${DOCKER_HOST:-$(shell docker context inspect --format '{{.Endpoints.docker.Host}}')} docker run --rm \
		-v "$(CURDIR)":/src -w /src \
		-v neomock-golangci-cache:/root/.cache/golangci-lint \
		$(LINT_IMAGE) golangci-lint run ./...

# CI локально через act (нужен Docker; настройки — .actrc).
# Событие pull_request: workflow запускается на любой ветке
# (у push-события фильтр branches: [main]).
# act не читает docker context — передаём эндпоинт текущего контекста;
# GITHUB_TOKEN из окружения убираем — act подставляет его при анонимном
# клонировании actions и получает «Invalid username or token».

DOCKER_ENV = DOCKER_HOST=$${DOCKER_HOST:-$(shell docker context inspect --format '{{.Endpoints.docker.Host}}')}

act:
	env -u GITHUB_TOKEN $(DOCKER_ENV) act pull_request

act-list:
	env -u GITHUB_TOKEN $(DOCKER_ENV) act --list
