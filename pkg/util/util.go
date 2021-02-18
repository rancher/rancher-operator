package util

import (
	"errors"

	"github.com/rancher/lasso/pkg/dynamic"
	"github.com/rancher/wrangler/pkg/data"
	"github.com/rancher/wrangler/pkg/data/convert"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/rancher/wrangler/pkg/summary"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func ToMap(obj runtime.Object) (data.Object, error) {
	if unstr, ok := obj.(*unstructured.Unstructured); ok {
		return unstr.Object, nil
	}
	return convert.EncodeToMap(obj)
}

func SetCondition(dynamic *dynamic.Controller, obj runtime.Object, conditionType string, err error) (runtime.Object, error) {
	var (
		reason  = ""
		status  = "True"
		message = ""
	)

	if errors.Is(generic.ErrSkip, err) {
		err = nil
	}

	if err != nil {
		reason = "Error"
		status = "False"
		message = err.Error()
	}

	desiredCondition := summary.NewCondition(conditionType, status, reason, message)

	data, mapErr := ToMap(obj)
	if mapErr != nil {
		return obj, mapErr
	}

	for _, condition := range summary.GetUnstructuredConditions(data) {
		if condition.Type() == conditionType {
			if desiredCondition.Equals(condition) {
				return obj, err
			}
			break
		}
	}

	data, mapErr = ToMap(obj.DeepCopyObject())
	if mapErr != nil {
		return obj, mapErr
	}

	conditions := data.Slice("status", "conditions")
	found := false
	for i, condition := range conditions {
		if condition.String("type") == conditionType {
			conditions[i] = desiredCondition.Object
			data.SetNested(conditions, "status", "conditions")
			found = true
		}
	}

	if !found {
		data.SetNested(append(conditions, desiredCondition.Object), "status", "conditions")
	}
	obj, updateErr := dynamic.UpdateStatus(&unstructured.Unstructured{
		Object: data,
	})
	if err != nil {
		return obj, err
	}
	return obj, updateErr
}
