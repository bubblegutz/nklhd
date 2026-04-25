app.readdir("/movies/{imdbID}/*", function(req)
    return {"metadata.txt", "poster.jpg", "subtitles/"}
end)

app.read("/movies/{imdbID}/metadata.txt", function(req)
    return "Title: Example Movie\nIMDB ID: " .. req.imdbID .. "\nYear: 2024"
end)

app.readdir("/movies/{imdbID}/subtitles/*", function(req)
    return {"en.srt", "es.srt", "fr.srt"}
end)

app.read("/movies/{imdbID}/subtitles/{lang}.srt", function(req)
    return "Subtitle content for " .. req.imdbID .. " in " .. req.lang
end)