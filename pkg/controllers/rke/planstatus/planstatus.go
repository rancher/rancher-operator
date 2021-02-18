package planstatus

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/rancher/rancher-operator/pkg/clients"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
)

type handler struct {
	secrets corecontrollers.SecretClient
}

func Register(ctx context.Context, clients *clients.Clients) {
	h := handler{
		secrets: clients.Core.Secret(),
	}
	clients.Core.Secret().OnChange(ctx, "plan-status", h.OnChange)
}

func (h *handler) OnChange(key string, secret *corev1.Secret) (*corev1.Secret, error) {
	if secret == nil || secret.Type != "rke.cattle.io/machine-plan" || len(secret.Data) == 0 {
		return secret, nil
	}

	appliedChecksum := string(secret.Data["applied-checksum"])
	plan := secret.Data["plan"]
	appliedPlan := secret.Data["appliedPlan"]

	if appliedChecksum == hash(plan) {
		if !bytes.Equal(plan, appliedPlan) {
			secret = secret.DeepCopy()
			secret.Data["appliedPlan"] = plan
			return h.secrets.Update(secret)
		}
	}

	return secret, nil
}

func hash(plan []byte) string {
	result := sha256.Sum256(plan)
	return hex.EncodeToString(result[:])
}
