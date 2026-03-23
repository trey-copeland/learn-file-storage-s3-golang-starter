package main

import (
	"encoding/base64"
	"encoding/json"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
)

func (cfg *apiConfig) normalizeThumbnailURL(video *database.Video) error {
	if video.ThumbnailURL == nil {
		return nil
	}

	current := *video.ThumbnailURL
	if strings.HasPrefix(current, "http://") || strings.HasPrefix(current, "https://") {
		if strings.Contains(current, "/assets/") {
			return nil
		}
		return nil
	}

	if strings.HasPrefix(current, "/assets/") {
		normalized := "http://localhost:" + cfg.port + current
		video.ThumbnailURL = &normalized
		return cfg.db.UpdateVideo(*video)
	}

	if strings.Contains(current, "/assets/") {
		idx := strings.Index(current, "/assets/")
		normalized := "http://localhost:" + cfg.port + current[idx:]
		video.ThumbnailURL = &normalized
		return cfg.db.UpdateVideo(*video)
	}

	if !strings.HasPrefix(current, "data:") {
		return nil
	}

	parts := strings.SplitN(current, ",", 2)
	if len(parts) != 2 {
		return nil
	}

	meta := strings.TrimPrefix(parts[0], "data:")
	if !strings.HasSuffix(meta, ";base64") {
		return nil
	}
	mediaType := strings.TrimSuffix(meta, ";base64")

	imgData, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}

	baseType, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		baseType = mediaType
	}

	ext := ".bin"
	if exts, err := mime.ExtensionsByType(baseType); err == nil && len(exts) > 0 {
		ext = exts[0]
	}

	imgName := video.ID.String() + ext
	imgPath := filepath.Join(cfg.assetsRoot, imgName)
	if err := os.WriteFile(imgPath, imgData, 0644); err != nil {
		return err
	}

	normalized := "http://localhost:" + cfg.port + "/assets/" + imgName
	video.ThumbnailURL = &normalized
	return cfg.db.UpdateVideo(*video)
}

func (cfg *apiConfig) handlerVideoMetaCreate(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		database.CreateVideoParams
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err = decoder.Decode(&params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode parameters", err)
		return
	}
	params.UserID = userID

	video, err := cfg.db.CreateVideo(params.CreateVideoParams)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create video", err)
		return
	}

	respondWithJSON(w, http.StatusCreated, video)
}

func (cfg *apiConfig) handlerVideoMetaDelete(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Couldn't get video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusForbidden, "You can't delete this video", err)
		return
	}

	err = cfg.db.DeleteVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't delete video", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (cfg *apiConfig) handlerVideoGet(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid video ID", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Couldn't get video", err)
		return
	}

	if err := cfg.normalizeThumbnailURL(&video); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't normalize thumbnail URL", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}

func (cfg *apiConfig) handlerVideosRetrieve(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	videos, err := cfg.db.GetVideos(userID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't retrieve videos", err)
		return
	}

	for i := range videos {
		if err := cfg.normalizeThumbnailURL(&videos[i]); err != nil {
			respondWithError(w, http.StatusInternalServerError, "Couldn't normalize thumbnail URL", err)
			return
		}
	}

	respondWithJSON(w, http.StatusOK, videos)
}
