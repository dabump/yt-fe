let downloadModal;
let wasProcessing = false;

document.addEventListener("DOMContentLoaded", () => {
    const modalEl = document.getElementById("downloadModal");
    const downloadForm = document.getElementById("downloadForm");
    const videoSearchInput = document.getElementById("videoSearch");

    if (modalEl) {
        downloadModal = new bootstrap.Modal(modalEl);
        modalEl.addEventListener("shown.bs.modal", () => {
            document.getElementById("url")?.focus();
        });
    }

    if (downloadForm) {
        downloadForm.addEventListener("submit", submitDownloadForm);
    }

    if (videoSearchInput) {
        videoSearchInput.addEventListener("input", filterVideos);
    }

    document.addEventListener("keydown", event => {
        if (event.key === "Escape") closeVideoBtn();
    });

    startStatusPolling();
});

function showDownloadModal() {
    downloadModal?.show();
}

function submitDownloadForm(event) {
    event.preventDefault();

    const form = event.currentTarget;
    const submitBtn = form.querySelector('button[type="submit"]');
    const urlInput = form.querySelector('input[name="url"]');
    const url = urlInput.value.trim();

    if (!url) return;

    submitBtn.disabled = true;
    submitBtn.textContent = "Adding...";
    downloadModal?.hide();

    const formData = new FormData();
    formData.append("url", url);

    fetch("/download", {
        method: "POST",
        body: formData,
    })
        .then(response => {
            urlInput.value = "";
            submitBtn.disabled = false;
            submitBtn.textContent = "Add to Queue";

            if (response.redirected) {
                window.location.href = response.url;
                return;
            }

            window.location.reload();
        })
        .catch(() => {
            submitBtn.disabled = false;
            submitBtn.textContent = "Add to Queue";
            downloadModal?.show();
        });
}

function openVideo(src) {
    const modalEl = document.getElementById("videoModal");
    const player = document.getElementById("videoPlayer");

    player.src = src;
    modalEl.classList.add("active");
    player.play();
}

function closeVideo(event) {
    if (event.target.id === "videoModal") {
        closeVideoBtn();
    }
}

function closeVideoBtn() {
    const modalEl = document.getElementById("videoModal");
    const player = document.getElementById("videoPlayer");

    if (!modalEl || !player) return;

    player.pause();
    player.src = "";
    modalEl.classList.remove("active");
}

function filterVideos() {
    const videoSearchInput = document.getElementById("videoSearch");
    const videoCards = document.querySelectorAll(".video-card");
    const videoCountEl = document.getElementById("videoCount");
    const searchTerms = videoSearchInput.value.toLowerCase().trim().split(/\s+/).filter(Boolean);
    let visibleCount = 0;

    videoCards.forEach(card => {
        const title = card.querySelector(".video-title").textContent.toLowerCase();
        const matches = searchTerms.length === 0 || searchTerms.some(term => title.includes(term));
        card.style.display = matches ? "" : "none";
        if (matches) visibleCount++;
    });

    const totalCount = videoCards.length;
    const filteredText = searchTerms.length > 0 ? `, ${visibleCount} showing` : "";
    videoCountEl.textContent = `${totalCount} videos${filteredText}`;
}

function deleteVideo(filename) {
    if (!confirm("Delete this video?")) return;

    fetch(`/delete/${filename}`, { method: "POST" })
        .then(response => {
            if (response.ok) {
                location.reload();
                return;
            }

            alert("Failed to delete video");
        });
}

function updateDownloadStatus() {
    fetch("/api/status")
        .then(response => response.json())
        .then(data => {
            const queueEl = document.getElementById("downloadQueue");
            const progressFill = document.getElementById("progressFill");
            const queueCount = document.getElementById("queueCount");
            const currentVideo = document.getElementById("currentVideo");

            if (data.processing || data.queue.length > 0) {
                queueEl.classList.add("visible");
                wasProcessing = true;

                if (data.processing && data.current) {
                    currentVideo.textContent = data.current.VideoID || data.current.video_id || "Preparing...";
                }

                const totalQueue = data.queue.length + (data.processing ? 1 : 0);
                queueCount.textContent = `${totalQueue} video${totalQueue > 1 ? "s" : ""} in queue`;

                const progress = data.processing ? data.progress : 0;
                progressFill.style.width = `${progress}%`;
                return;
            }

            queueEl.classList.remove("visible");
            if (wasProcessing) {
                wasProcessing = false;
                setTimeout(() => location.reload(), 500);
            }
        })
        .catch(error => console.log("Status poll error:", error));
}

function startStatusPolling() {
    updateDownloadStatus();
    setInterval(updateDownloadStatus, 1000);
}
