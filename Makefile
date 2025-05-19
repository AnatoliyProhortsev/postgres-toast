build-toasted: clean go-init build-toasted build-app build-workload
build-original: clean go-init build-original build-app build-workload

clean:
	docker-compose down --remove-orphans --volumes

build-toasted:
	docker build -f db/postgres_dockerfile --build-arg PG_REPO=https://github.com/postgrespro/postgres.git --build-arg PG_BRANCH=jsonb_toaster -t my_postgres db

build-original:
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
	docker-compose stop workload
	docker-compose build workload
	docker-compose up -d workload

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