.PHONY: start stop restart logs ps

# Start docker containers
start:
	docker-compose up -d

# Stop docker containers
stop:
	docker-compose down

# Restart docker containers
restart: stop start

# View logs
logs:
	docker-compose logs -f

# Check status
ps:
	docker-compose ps
