build-toasted: clean go-init build-postgres-toasted build-app build-workload run
build-original: clean go-init build-postgres-original build-app build-workload run

clean:
	docker-compose down --remove-orphans --volumes

build-postgres-toasted:
	docker build -f db/postgres_dockerfile --build-arg PG_REPO=https://github.com/postgrespro/postgres.git --build-arg PG_BRANCH=jsonb_toaster -t my_postgres db

build-postgres-original:
	docker build -f db/postgres_dockerfile --build-arg PG_REPO=https://github.com/postgrespro/postgres.git --build-arg PG_BRANCH=REL_17_STABLE -t my_postgres db

build-app:
	docker build -f app/service_dockerfile -t my_app app

build-workload:
	docker build -f workload/workloader_dockerfile -t my_workload workload

reinstall-app:
	docker-compose stop app
	docker-compose build app
	docker-compose up -d app

reinstall-workload:
	docker-compose stop loadgen
	docker-compose build loadgen
	docker-compose up -d loadgen

reinstall-postgres-toasted:
	docker-compose stop my_postgres
	docker rm my_postgres
	docker build -f db/postgres_dockerfile --build-arg PG_REPO=https://github.com/postgrespro/postgres.git --build-arg PG_BRANCH=jsonb_toaster -t my_postgres db
	docker-compose up -d my_postgres

reinstall-postgres-original:
	docker-compose stop postgres
	docker rm postgres
	docker build -f db/postgres_dockerfile --build-arg PG_REPO=https://github.com/postgrespro/postgres.git --build-arg PG_BRANCH=REL_17_STABLE -t my_postgres db
	docker-compose up -d postgres

save-app:
	docker save -o app.tar my_app

save-postgres:
	docker save -o postgres.tar my_postgres

load-app:
	docker load -i app.tar

load-postgres:
	docker load -i postgres.tar

go-init:
	cd app && rm -f go.mod && go mod init app && go mod tidy
	cd monitor && rm -f go.mod && go mod init monitor && go mod tidy
	cd workload && rm -f go.mod && go mod init workload && go mod tidy

run:
	docker-compose up --force-recreate
