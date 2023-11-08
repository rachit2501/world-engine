###############################################################################
###                                CI	                                    ###
###############################################################################

include makefiles/ci.mk

include makefiles/test.mk

###############################################################################
###                                Build                                    ###
###############################################################################

include makefiles/build.mk

###############################################################################
###                            Docker Utils                                 ###
###############################################################################

kill:
	docker kill $$(docker ps -q)
