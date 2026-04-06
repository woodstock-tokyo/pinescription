# SPDX-FileCopyrightText: 2026 Woodstock K.K.
#
# SPDX-License-Identifier: AGPL-3.0-only

.PHONY: addlicense

ADDLICENSE ?= reuse
COPYRIGHT_HOLDER ?= Woodstock K.K.
LICENSE_TYPE ?= AGPL-3.0-only

addlicense:
	find . -type f -name '*.go' -print0 | xargs -0 $(ADDLICENSE) annotate \
		--copyright "$(COPYRIGHT_HOLDER)" --license "$(LICENSE_TYPE)"
	$(ADDLICENSE) annotate --copyright "$(COPYRIGHT_HOLDER)" --license "$(LICENSE_TYPE)" go.mod go.sum .gitignore .pre-commit-config.yaml Makefile README.md SECURITY.md .github/workflows/pr-main.yml
