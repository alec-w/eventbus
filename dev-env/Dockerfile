FROM debian:12.12-slim

# Create a user that will be used when inisde the container rather than root
RUN useradd dev --create-home && \
    echo "dev:pass" | chpasswd && \
    adduser dev sudo

# bash-completion, gcc, make, vim and wget are useful development tools
# curl is needed for installation of Go and other tools
# git is needed to run commands such as go mod tidy during development
# unzip is needed to unzip downloaded tools
# sudo allows installing additional packages locally without requiring an image rebuild
RUN apt-get update && apt-get -y install \
    bash-completion \
    curl \
    gcc \
    git \
    make \
    unzip \
    vim \
    wget \
    sudo

# Install Go
ARG GO_VERSION=1.25.0
# Modify PATH to make go immediately available after installation
ENV PATH=$PATH:/usr/local/go/bin
RUN curl -sSfL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xzf -  && \
    go env -w GOBIN=/usr/local/go/bin GOROOT=/usr/local/go && \
    echo "export GOROOT=/usr/local/go" > /etc/profile.d/go.sh  && \
    echo "export PATH=\$PATH:/usr/local/go/bin" >> /etc/profile.d/go.sh

# Install Go linter
ARG GOLANGCI_VERSION=2.4.0
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOROOT)/bin v${GOLANGCI_VERSION}

CMD ["sh"]
ENTRYPOINT ["bash", "-lec"]
