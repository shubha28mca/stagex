# Convenience wrapper around docker compose for IIG StageX.
# Usage:  make up | make down | make logs | make ps | make restart | make reset
COMPOSE ?= docker compose

.PHONY: up down restart logs ps reset

up:        ## Build and start the whole stack in the background
	$(COMPOSE) up --build -d

down:      ## Stop and remove containers (keeps the database volume)
	$(COMPOSE) down

restart:   ## Rebuild and restart
	$(COMPOSE) up --build -d

logs:      ## Follow logs from all services
	$(COMPOSE) logs -f

ps:        ## Show service status
	$(COMPOSE) ps

reset:     ## Stop and DELETE all data (database + uploaded media)
	$(COMPOSE) down -v
