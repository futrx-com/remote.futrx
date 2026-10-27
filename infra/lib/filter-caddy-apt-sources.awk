# Filter only active source URIs for Caddy's stable Cloudsmith repository.
# Keep unrelated repositories, source options, and Signed-By fields intact.
function is_caddy(uri) {
    return uri ~ /^https?:\/\/dl[.]cloudsmith[.]io\/public\/caddy\/stable\/deb\/debian\/?$/
}
BEGIN {
    if (format == "sources") {
        RS = ""
        ORS = "\n\n"
    }
}
format == "list" {
    if ($0 ~ /^[[:space:]]*deb(-src)?[[:space:]]/) {
        line = $0
        sub(/^[[:space:]]+/, "", line)
        count = split(line, fields, /[[:space:]]+/)
        uri_index = 2
        if (fields[uri_index] ~ /^\[/) {
            while (uri_index <= count && fields[uri_index] !~ /\]$/) uri_index++
            uri_index++
        }
        if (is_caddy(fields[uri_index])) {
            print "removed" >> removed_file
            next
        }
    }
    print
    next
}
format == "sources" {
    count = split($0, lines, "\n")
    uri_start = 0
    uri_end = 0
    uri_open = 0
    uris = ""
    for (i = 1; i <= count; i++) {
        if (lines[i] ~ /^[ \t]*#/) continue
        if (tolower(lines[i]) ~ /^uris[[:space:]]*:/) {
            uri_start = i
            uri_end = i
            uri_open = 1
            uris = lines[i]
            sub(/^[^:]*:[[:space:]]*/, "", uris)
        } else if (uri_open && lines[i] ~ /^[ \t]/) {
            uri_end = i
            uris = uris " " lines[i]
        } else uri_open = 0
    }
    removed = 0
    remaining = ""
    uri_count = split(uris, fields, /[[:space:]]+/)
    for (i = 1; i <= uri_count; i++) {
        if (is_caddy(fields[i])) removed++
        else if (fields[i] != "") remaining = remaining (remaining ? " " : "") fields[i]
    }
    if (!removed) {
        print
        next
    }
    print "removed" >> removed_file
    if (!remaining) next
    stanza = ""
    for (i = 1; i <= count; i++) {
        if (i == uri_start) value = "URIs: " remaining
        else if (i > uri_start && i <= uri_end && lines[i] !~ /^[ \t]*#/) continue
        else value = lines[i]
        stanza = stanza (stanza ? "\n" : "") value
    }
    print stanza
}
