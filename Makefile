build-toasted:
	docker build -f docker/postgres_dockerfile --build-arg PG_REPO=https://github.com/postgrespro/postgres.git --build-arg PG_BRANCH=jsonb_toaster -t my-postgres docker
	docker run -d -p 5432:5432 my-postgres

build-original:
	docker build -f docker/postgres_dockerfile --build-arg PG REPO=https://github.com/postgrespro/postgres.git --build-arg PG_BRANCH=REL_17_STABLE -t my-postgres docker
	docker run -d -p 5432:5432 my-postgres