FROM scratch

COPY build/proplet /exe

ENTRYPOINT ["/exe"]
