#!/bin/bash
TABLE=$1
BATCH=500
OFFSET=0
TOTAL=0

while true; do
  DATA=$(ssh -i /tmp/safepc-key.pem -o StrictHostKeyChecking=no rocky@43.202.241.121 "sudo podman exec timescaledb-safepc psql -U postgres -d timescale_db -t -A -c \"SELECT json_build_object(pk, log_pk, t, time, h, hostname, m, msg_id, s, severity, c, cef_extensions) FROM $TABLE ORDER BY log_pk LIMIT $BATCH OFFSET $OFFSET;\"" 2>/dev/null)
  
  [ -z "$DATA" ] && break
  
  TMPFILE=$(mktemp)
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    TS=$(echo "$line" | jq -r ".t[:10]" | tr "-" ".")
    DOC=$(echo "$line" | jq -c "{\"@timestamp\": .t, \"log_pk\": .pk, \"msg_id\": .m, \"severity\": .s, \"hostname\": .h, \"event_time\": .c.rt, \"user_id\": .c.suid, \"os_account\": .c.suser, \"user_ip\": .c.src, \"user_mac\": .c.smac, \"action\": .c.act, \"outcome\": .c.outcome, \"file_name\": .c.fname, \"file_path\": .c.filePath}")
    echo "{\"index\":{\"_index\":\"safepc-logs-$TS\"}}" >> $TMPFILE
    echo "$DOC" >> $TMPFILE
  done <<< "$DATA"
  
  curl -s -X POST "http://localhost:49200/_bulk" -H "Content-Type: application/json" --data-binary @$TMPFILE > /dev/null
  rm -f $TMPFILE
  
  COUNT=$(echo "$DATA" | grep -c "^{")
  TOTAL=$((TOTAL + COUNT))
  echo "$TABLE: $TOTAL"
  
  [ $COUNT -lt $BATCH ] && break
  OFFSET=$((OFFSET + BATCH))
done
echo "Done: $TOTAL"
