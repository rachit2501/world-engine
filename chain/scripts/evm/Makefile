deploy:
	forge create Game -r localhost:8545 --interactive

make join:
	cast call $(filter-out $@,$(MAKECMDGOALS)) "joinGame()(bool)" --rpc-url localhost:8545
