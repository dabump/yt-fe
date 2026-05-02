let downloadModal;
let tagManagerModal;
let videoTagsModal;
let wasProcessing = false;
let formatsLoadedForURL = "";
let allTags = [];
let activeTagFilters = new Set();

document.addEventListener("DOMContentLoaded", () => {
    const modalEl = document.getElementById("downloadModal");
    const tagManagerEl = document.getElementById("tagManagerModal");
    const videoTagsEl = document.getElementById("videoTagsModal");
    const downloadForm = document.getElementById("downloadForm");
    const tagCreateForm = document.getElementById("tagCreateForm");
    const videoSearchInput = document.getElementById("videoSearch");

    if (modalEl) {
        downloadModal = new bootstrap.Modal(modalEl);
        modalEl.addEventListener("shown.bs.modal", () => {
            document.getElementById("url")?.focus();
        });
        modalEl.addEventListener("hidden.bs.modal", resetDownloadForm);
    }

    if (tagManagerEl) {
        tagManagerModal = new bootstrap.Modal(tagManagerEl);
    }

    if (videoTagsEl) {
        videoTagsModal = new bootstrap.Modal(videoTagsEl);
    }

    if (downloadForm) {
        downloadForm.addEventListener("submit", submitDownloadForm);
        downloadForm.querySelector('input[name="url"]')?.addEventListener("input", resetFormatSelection);
    }

    if (tagCreateForm) {
        tagCreateForm.addEventListener("submit", createTag);
    }

    if (videoSearchInput) {
        videoSearchInput.addEventListener("input", filterVideos);
    }

    document.addEventListener("keydown", event => {
        if (event.key === "Escape") closeVideoBtn();
    });

    loadTags();
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
    const formatSelectorInput = document.getElementById("formatSelector");
    const resolutionInput = document.getElementById("resolution");
    const formatOptions = document.getElementById("formatOptions");
    const url = urlInput.value.trim();

    if (!url) return;

    if (formatsLoadedForURL !== url || !formatSelectorInput.value) {
        checkAvailableFormats(url, submitBtn);
        return;
    }

    const selectedOption = formatOptions.selectedOptions[0];
    formatSelectorInput.value = selectedOption.value;
    resolutionInput.value = selectedOption.dataset.label;

    const formData = new FormData();
    formData.append("url", url);
    formData.append("format_selector", formatSelectorInput.value);
    formData.append("resolution", resolutionInput.value);

    submitBtn.disabled = true;
    submitBtn.textContent = "Adding...";
    downloadModal?.hide();

    fetch("/download", {
        method: "POST",
        body: formData,
    })
        .then(response => {
            urlInput.value = "";
            submitBtn.disabled = false;
            submitBtn.textContent = "Check Resolutions";
            resetFormatSelection();

            if (response.redirected) {
                window.location.href = response.url;
                return;
            }

            window.location.reload();
        })
        .catch(() => {
            submitBtn.disabled = false;
            submitBtn.textContent = "Check Resolutions";
            downloadModal?.show();
        });
}

function checkAvailableFormats(url, submitBtn) {
    const formatError = document.getElementById("formatError");
    const formData = new FormData();
    formData.append("url", url);

    submitBtn.disabled = true;
    submitBtn.textContent = "Checking...";
    formatError.hidden = true;
    formatError.textContent = "";

    fetch("/api/formats", {
        method: "POST",
        body: formData,
    })
        .then(async response => {
            if (!response.ok) {
                throw new Error(await response.text());
            }
            return response.json();
        })
        .then(data => {
            renderFormatOptions(url, data.formats || []);
            submitBtn.disabled = false;
            submitBtn.textContent = "Add to Queue";
        })
        .catch(error => {
            submitBtn.disabled = false;
            submitBtn.textContent = "Check Resolutions";
            formatError.textContent = error.message || "Failed to load available resolutions.";
            formatError.hidden = false;
        });
}

function renderFormatOptions(url, formats) {
    const formatPicker = document.getElementById("formatPicker");
    const formatOptions = document.getElementById("formatOptions");
    const formatSelectorInput = document.getElementById("formatSelector");
    const resolutionInput = document.getElementById("resolution");
    const formatHelp = document.getElementById("formatHelp");

    formatOptions.innerHTML = "";

    formats.forEach(format => {
        const option = document.createElement("option");
        option.value = format.format_selector;
        option.textContent = format.label;
        option.dataset.label = format.label;
        formatOptions.appendChild(option);
    });

    if (formats.length === 0) {
        throw new Error("No downloadable video resolutions were found for this URL.");
    }

    formatsLoadedForURL = url;
    formatSelectorInput.value = formats[0].format_selector;
    resolutionInput.value = formats[0].label;
    formatHelp.textContent = "Choose Best available, 4K, 2K, or any lower resolution exposed by YouTube for this video.";
    formatPicker.hidden = false;
}

function resetDownloadForm() {
    const form = document.getElementById("downloadForm");
    form?.reset();
    resetFormatSelection();
}

function resetFormatSelection() {
    const formatPicker = document.getElementById("formatPicker");
    const formatOptions = document.getElementById("formatOptions");
    const formatSelectorInput = document.getElementById("formatSelector");
    const resolutionInput = document.getElementById("resolution");
    const formatError = document.getElementById("formatError");
    const submitBtn = document.getElementById("downloadSubmit");

    formatsLoadedForURL = "";
    if (formatPicker) formatPicker.hidden = true;
    if (formatOptions) formatOptions.innerHTML = "";
    if (formatSelectorInput) formatSelectorInput.value = "";
    if (resolutionInput) resolutionInput.value = "";
    if (formatError) {
        formatError.hidden = true;
        formatError.textContent = "";
    }
    if (submitBtn) {
        submitBtn.disabled = false;
        submitBtn.textContent = "Check Resolutions";
    }
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
        const tags = (card.dataset.tags || "").split(",").filter(Boolean);
        const matches = searchTerms.length === 0 || searchTerms.some(term => title.includes(term));
        const matchesTags = [...activeTagFilters].every(tag => tags.includes(tag.toLowerCase()));
        card.style.display = matches && matchesTags ? "" : "none";
        if (matches && matchesTags) visibleCount++;
    });

    const totalCount = videoCards.length;
    const filteredText = searchTerms.length > 0 ? `, ${visibleCount} showing` : "";
    videoCountEl.textContent = `${totalCount} videos${filteredText}`;
}

function toggleTagFilter(button) {
    const tag = button.dataset.tag.toLowerCase();
    if (activeTagFilters.has(tag)) {
        activeTagFilters.delete(tag);
        button.classList.remove("active");
    } else {
        activeTagFilters.add(tag);
        button.classList.add("active");
    }
    filterVideos();
}

function openTagManager() {
    clearTagErrors();
    loadTags().then(() => tagManagerModal?.show());
}

function loadTags() {
    return fetch("/api/tags")
        .then(response => response.json())
        .then(data => {
            allTags = data.tags || [];
            renderTagManager();
            renderTagFilters();
            return allTags;
        })
        .catch(error => {
            showTagManagerError(error.message || "Failed to load tags.");
            return [];
        });
}

function createTag(event) {
    event.preventDefault();
    const input = document.getElementById("newTagName");
    const name = input.value.trim();
    if (!name) return;

    fetch("/api/tags", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name }),
    })
        .then(handleTagResponse)
        .then(data => {
            input.value = "";
            allTags = data.tags || [];
            renderTagManager();
            renderTagFilters();
            clearTagErrors();
        })
        .catch(error => showTagManagerError(error.message));
}

function renameTag(oldName) {
    const name = prompt("Rename tag", oldName);
    if (!name || name.trim() === oldName) return;

    fetch(`/api/tags/${encodeURIComponent(oldName)}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: name.trim() }),
    })
        .then(handleTagResponse)
        .then(data => {
            allTags = data.tags || [];
            activeTagFilters.clear();
            renderTagManager();
            renderTagFilters();
            clearTagErrors();
            location.reload();
        })
        .catch(error => showTagManagerError(error.message));
}

function deleteTag(name) {
    if (!confirm(`Delete tag "${name}" and remove it from all videos?`)) return;

    fetch(`/api/tags/${encodeURIComponent(name)}`, { method: "DELETE" })
        .then(handleTagResponse)
        .then(data => {
            allTags = data.tags || [];
            activeTagFilters.delete(name.toLowerCase());
            renderTagManager();
            renderTagFilters();
            clearTagErrors();
            location.reload();
        })
        .catch(error => showTagManagerError(error.message));
}

function renderTagManager() {
    const tagList = document.getElementById("tagList");
    if (!tagList) return;

    tagList.innerHTML = "";
    if (allTags.length === 0) {
        tagList.innerHTML = '<p class="format-help">No tags yet. Create one to start organizing videos.</p>';
        return;
    }

    allTags.forEach(tag => {
        const row = document.createElement("div");
        row.className = "tag-row";
        row.innerHTML = `
            <span>${escapeHTML(tag)}</span>
            <div>
                <button type="button" class="secondary-action" onclick="renameTag('${escapeJS(tag)}')">Rename</button>
                <button type="button" class="secondary-action danger" onclick="deleteTag('${escapeJS(tag)}')">Delete</button>
            </div>
        `;
        tagList.appendChild(row);
    });
}

function renderTagFilters() {
    const tagFilter = document.getElementById("tagFilter");
    if (!tagFilter) return;

    tagFilter.innerHTML = "";
    allTags.forEach(tag => {
        const button = document.createElement("button");
        button.type = "button";
        button.className = "tag-filter-chip";
        button.dataset.tag = tag;
        button.textContent = tag;
        if (activeTagFilters.has(tag.toLowerCase())) {
            button.classList.add("active");
        }
        button.addEventListener("click", () => toggleTagFilter(button));
        tagFilter.appendChild(button);
    });
    filterVideos();
}

function openVideoTags(button) {
    const filename = button.dataset.filename;
    const title = button.dataset.title;
    document.getElementById("videoTagsFilename").value = filename;
    document.getElementById("videoTagsTitle").textContent = `Tags for ${title}`;
    clearVideoTagError();

    Promise.all([
        loadTags(),
        fetch(`/api/video-tags/${encodeURIComponent(filename)}`).then(response => response.json()),
    ])
        .then(([, data]) => {
            renderVideoTagChoices(data.tags || []);
            videoTagsModal?.show();
        })
        .catch(error => showVideoTagError(error.message || "Failed to load video tags."));
}

function renderVideoTagChoices(selectedTags) {
    const list = document.getElementById("videoTagsList");
    list.innerHTML = "";

    if (allTags.length === 0) {
        list.innerHTML = '<p class="format-help">No tags exist yet. Create tags from the Tags button first.</p>';
        return;
    }

    const selected = new Set(selectedTags.map(tag => tag.toLowerCase()));
    allTags.forEach(tag => {
        const label = document.createElement("label");
        label.className = "tag-checkbox";
        label.innerHTML = `
            <input type="checkbox" value="${escapeHTML(tag)}" ${selected.has(tag.toLowerCase()) ? "checked" : ""}>
            <span>${escapeHTML(tag)}</span>
        `;
        list.appendChild(label);
    });
}

function saveVideoTags() {
    const filename = document.getElementById("videoTagsFilename").value;
    const selectedTags = [...document.querySelectorAll('#videoTagsList input[type="checkbox"]:checked')].map(input => input.value);

    fetch(`/api/video-tags/${encodeURIComponent(filename)}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ tags: selectedTags }),
    })
        .then(handleTagResponse)
        .then(() => location.reload())
        .catch(error => showVideoTagError(error.message));
}

function handleTagResponse(response) {
    if (!response.ok) {
        return response.text().then(message => {
            throw new Error(message || "Tag request failed.");
        });
    }
    return response.json();
}

function showTagManagerError(message) {
    const error = document.getElementById("tagManagerError");
    if (!error) return;
    error.textContent = message;
    error.hidden = false;
}

function clearTagErrors() {
    const error = document.getElementById("tagManagerError");
    if (!error) return;
    error.textContent = "";
    error.hidden = true;
}

function showVideoTagError(message) {
    const error = document.getElementById("videoTagsError");
    if (!error) return;
    error.textContent = message;
    error.hidden = false;
}

function clearVideoTagError() {
    const error = document.getElementById("videoTagsError");
    if (!error) return;
    error.textContent = "";
    error.hidden = true;
}

function escapeHTML(value) {
    return value.replace(/[&<>"]/g, char => ({
        "&": "&amp;",
        "<": "&lt;",
        ">": "&gt;",
        '"': "&quot;",
    })[char]);
}

function escapeJS(value) {
    return value.replace(/\\/g, "\\\\").replace(/'/g, "\\'");
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
