build-toasted:
	docker build --build-arg PG_REPO=https://github.com/postgrespro/postgres.git --build-arg PG_BRANCH=jsonb_toaster -t my-postgres docker

build-original:
	docker build --build-arg PG REPO=https://github.com/postgres/postgres.git --build-arg PG_BRANCH=main -t my-postgres docker