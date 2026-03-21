.PHONY: run build install-deps clean

run: build
	./yt-fe

build:
	go build -o yt-fe

install-deps:
	go mod download
	@echo "Note: You also need yt-dlp and ffmpeg installed:"
	@echo "  pip install yt-dlp"
	@echo "  apt install ffmpeg  # or brew install ffmpeg on macOS"

clean:
	rm -f yt-fe
