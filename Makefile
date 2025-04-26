toasted: clean build-toasted build-app run
original: clean build-original build-app save-postgres save-app run

clean:
	docker-compose down --remove-orphans --volumes

build-toasted:
	docker build -f db/postgres_dockerfile --build-arg PG_REPO=https://github.com/postgrespro/postgres.git --build-arg PG_BRANCH=jsonb_toaster -t my_postgres db

build-original:
	docker build -f db/postgres_dockerfile --build-arg PG REPO=https://github.com/postgrespro/postgres.git --build-arg PG_BRANCH=REL_17_STABLE -t my_postgres db

build-app:
	docker build -f app/service_dockerfile -t my_app app

save-app:
	docker save -o app.tar my_app

save-postgres:
	docker save -o postgres.tar my_postgres

load-app:
	docker load -i app.tar

load-postgres:
	docker load -i postgres.tar

run:
	docker-compose up --force-recreate