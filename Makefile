toasted: clean build-toasted run
original: clean build-original run

clean:
	docker-compose down --remove-orphans --volumes

build-toasted:
	docker build -f docker/postgres_dockerfile --build-arg PG_REPO=https://github.com/postgrespro/postgres.git --build-arg PG_BRANCH=jsonb_toaster -t my-postgres docker

build-original:
	docker build -f docker/postgres_dockerfile --build-arg PG REPO=https://github.com/postgrespro/postgres.git --build-arg PG_BRANCH=REL_17_STABLE -t my-postgres docker

run:
	docker-compose up --force-recreate