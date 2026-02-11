TAPES := $(wildcard *.tape)
GIFS  := $(TAPES:%.tape=../docs/assets/screenshots/%.gif)

.PHONY: all clean list

all: $(GIFS) ## Generate all screenshots
	@echo "Generated screenshots in docs/assets/screenshots/"

../docs/assets/screenshots/%.gif: %.tape
	@mkdir -p ../docs/assets/screenshots
	vhs $<

clean: ## Remove generated screenshots
	rm -f ../docs/assets/screenshots/wt-*.gif ../docs/assets/screenshots/wt-*.webm

list: ## Show available tapes
	@echo "Available tapes:"
	@for f in $(TAPES); do echo "  $$f"; done

# Individual tape targets for convenience
.PHONY: help-tape list-tape create-tape config-tape workflow-tape
help-tape: wt-help.tape
	vhs $<
list-tape: wt-list.tape
	vhs $<
create-tape: wt-create.tape
	vhs $<
config-tape: wt-config.tape
	vhs $<
workflow-tape: wt-workflow.tape
	vhs $<
