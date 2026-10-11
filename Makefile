# Convenience wrapper around docker compose for IIG StageX.
# Usage:  make up | make down | make logs | make ps | make restart | make reset
#         make vm-up | make vm-down   (macOS 12-13 only, see README)
COMPOSE ?= docker compose

# Colima VM sizing, used only by `make vm-up` on macOS 12/13 where Docker
# Desktop cannot run. Override on a roomier machine: make vm-up COLIMA_MEMORY=8
COLIMA_CPU    ?= 2
COLIMA_MEMORY ?= 4
COLIMA_DISK   ?= 40

.PHONY: up down restart logs ps reset vm-up vm-down build-serial

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

build-serial: ## Build the images one at a time (avoids OOM on low-RAM machines)
	for s in api web admin-api admin-web; do \
	  echo "--- building $$s ---"; $(COMPOSE) build $$s || exit 1; \
	done

vm-up:     ## macOS 12-13: start the Colima VM that provides the Docker engine
	colima status >/dev/null 2>&1 \
	  || colima start --cpu $(COLIMA_CPU) --memory $(COLIMA_MEMORY) --disk $(COLIMA_DISK)

vm-down:   ## macOS 12-13: stop the Colima VM and release its RAM
	colima stop
