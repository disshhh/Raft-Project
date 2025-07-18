1. Start 3 Nodes (in separate terminals)
bash
Copy
# Terminal 1 (node1)
go run main.go node1 127.0.0.1:12000 8080

# Terminal 2 (node2)
go run main.go node2 127.0.0.1:12001 8081

# Terminal 3 (node3)
go run main.go node3 127.0.0.1:12002 8082
2. Find Current Leader
bash
Copy
# Check all nodes' status
curl http://localhost:8080/status
curl http://localhost:8081/status 
curl http://localhost:8082/status

# Leader will show "Leader", others show "Follower"
#Leader will show "Leader", others show "Follower" if not then run:
# Assuming node3 (8082) is leader, join new nodes:
curl -X POST http://localhost:8082/join \
  -H "Content-Type: application/json" \
  -d '{"id":"node1","address":"127.0.0.1:12000"}'

curl -X POST http://localhost:8082/join \
  -H "Content-Type: application/json" \
  -d '{"id":"node2","address":"127.0.0.1:12001"}'
# Leader will show "Leader", others show "Follower"

3. Verify Cluster Membership
bash
Copy
# On each node (should show identical data)
curl http://localhost:8080/api/v1/printers
curl http://localhost:8081/api/v1/printers
curl http://localhost:8082/api/v1/printers
4. Test All Required Endpoints
Create Printer (on leader)
bash
Copy
curl -X POST http://localhost:<LEADER_PORT>/api/v1/printers \
  -H "Content-Type: application/json" \
  -d '{"id":"printer1","company":"Creality","model":"Ender3"}'
List Printers
bash
Copy
curl http://localhost:8080/api/v1/printers
Create Filament
bash
Copy
curl -X POST http://localhost:<LEADER_PORT>/api/v1/filaments \
  -H "Content-Type: application/json" \
  -d '{"id":"filament1","type":"PLA","color":"Red","total_weight_in_grams":1000}'
List Filaments
bash
Copy
curl http://localhost:8080/api/v1/filaments
Create Print Job
bash
Copy
curl -X POST http://localhost:<LEADER_PORT>/api/v1/print-jobs \
  -H "Content-Type: application/json" \
  -d '{
    "id":"job1",
    "printer_id":"printer1",
    "filament_id":"filament1",
    "print_weight_in_grams":200
  }'
List Print Jobs
bash
Copy
curl http://localhost:8080/api/v1/print-jobs
Update Job Status (Valid Transition)
bash
Copy
# Queued → Running
curl -X POST "http://localhost:<LEADER_PORT>/api/v1/print-jobs/job1/status?status=running"

# Running → Done (deducts filament)
curl -X POST "http://localhost:<LEADER_PORT>/api/v1/print-jobs/job1/status?status=done"
Test Invalid Transition
bash
Copy
# Try invalid Queued → Done
curl -X POST "http://localhost:<LEADER_PORT>/api/v1/print-jobs/job1/status?status=done"
# Should return error
5. Verify Fault Tolerance
bash
Copy
# 1. Kill current leader (Ctrl+C in its terminal)
# 2. Check new leader election (within 5-10 seconds):
curl http://localhost:8080/status
curl http://localhost:8081/status
curl http://localhost:8082/status

# 3. Verify data persists through leader change
curl http://localhost:<NEW_LEADER_PORT>/api/v1/printers
6. Verify Filament Deduction
bash
Copy
# After completing a job (status=done), check:
curl http://localhost:8080/api/v1/filaments
# remaining_weight_in_grams should decrease by print weight
