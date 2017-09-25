PKG_TEST = gopkg.in/dedis/onet.test
PKG_STABLE = gopkg.in/dedis/onet.v1
CREATE_STABLE = $$GOPATH/src/github.com/dedis/Coding/bin/create_stable.sh -o stable

all: test

test_fmt:
	@echo Checking correct formatting of files
	@{ \
		files=$$( go fmt ./... ); \
		if [ -n "$$files" ]; then \
		echo "Files not properly formatted: $$files"; \
		exit 1; \
		fi; \
		if ! go vet ./...; then \
		exit 1; \
		fi \
	}

test_lint:
	@echo Checking linting of files
	@{ \
		go get -u github.com/golang/lint/golint; \
		exclude="protocols/byzcoin|_test.go"; \
		lintfiles=$$( golint ./... | egrep -v "($$exclude)" ); \
		if [ -n "$$lintfiles" ]; then \
		echo "Lint errors:"; \
		echo "$$lintfiles"; \
		exit 1; \
		fi \
	}

# You can use `test_playground` to run any test or part of onet
# for more than once in Travis. Change `make test` in .travis.yml
# to `make test_playground`.
test_playground:
	cd network; \
	for a in $$( seq 10 ); do \
	  go test -v -race -short || exit 1 ; \
	done;

test_verbose:
	go test -p=1 -v -race -short ./...

test_goveralls:
	./coveralls.sh
	$$GOPATH/bin/goveralls -coverprofile=profile.cov -service=travis-ci

test_stable_build:
	$(CREATE_STABLE) $(PKG_TEST)
	cd $$GOPATH/src/$(PKG_TEST); go build ./...

test_stable:
	$(CREATE_STABLE) $(PKG_TEST)
	cd $$GOPATH/src/$(PKG_TEST); make test

test: test_fmt test_lint test_goveralls test_stable_build

create_stable:
	$(CREATE_STABLE) $(PKG_STABLE)
