FROM nvidia/cuda:12.0.0-devel-ubuntu22.04
RUN apt update
RUN curl -fsSL https://deb.nodesource.com/setup_20.x | bash - &&\
    apt install -y nodejs &&\
    wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz &&\
    rm -rf /usr/local/go &&\
    tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
RUN export PATH=$PATH:/usr/local/go/bin