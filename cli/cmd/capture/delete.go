// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package capture

import (
	"context"
	"fmt"

	retinacmd "github.com/microsoft/retina/cli/cmd"
	captureConstants "github.com/microsoft/retina/pkg/capture/constants"
	"github.com/microsoft/retina/pkg/label"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
)

var name string

var deleteExample = templates.Examples(i18n.T(`
		# Delete the Retina Capture "retina-capture-8v6wd" in namespace "capture"
		kubectl retina capture delete --name retina-capture-8v6wd --namespace capture
		`))

func CaptureCmdDelete() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete",
		Short:   "Delete a Retina capture",
		Example: deleteExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			kubeConfig, err := configFlags.ToRESTConfig()
			if err != nil {
				return err
			}

			kubeClient, err := kubernetes.NewForConfig(kubeConfig)
			if err != nil {
				return err
			}

			if len(name) == 0 {
				return fmt.Errorf("Capture name is not specified")
			}

			captureJobSelector := &metav1.LabelSelector{
				MatchLabels: map[string]string{
					label.CaptureNameLabel: name,
					label.AppLabel:         captureConstants.CaptureAppname,
				},
			}
			labelSelector, _ := labels.Parse(metav1.FormatLabelSelector(captureJobSelector))
			jobListOpt := metav1.ListOptions{
				LabelSelector: labelSelector.String(),
			}

			jobList, err := kubeClient.BatchV1().Jobs(namespace).List(context.TODO(), jobListOpt)
			if err != nil {
				return err
			}
			if len(jobList.Items) == 0 {
				return fmt.Errorf("Capture %s in namespace %s is not found", name, namespace)
			}

			for _, job := range jobList.Items {
				deletePropagationBackground := metav1.DeletePropagationBackground
				if err := kubeClient.BatchV1().Jobs(job.Namespace).Delete(context.TODO(), job.Name, metav1.DeleteOptions{
					PropagationPolicy: &deletePropagationBackground,
				}); err != nil {
					retinacmd.Logger.Info("Failed to delete job", zap.String("job name", job.Name), zap.Error(err))
				}
			}

			for _, volume := range jobList.Items[0].Spec.Template.Spec.Volumes {
				if volume.Secret != nil {
					if err := kubeClient.CoreV1().Secrets(namespace).Delete(context.TODO(), volume.Secret.SecretName, metav1.DeleteOptions{}); err != nil {
						return err
					}
					break
				}
			}
			retinacmd.Logger.Info(fmt.Sprintf("Retina Capture %q delete", name))

			return nil
		},
	}

	configFlags = genericclioptions.NewConfigFlags(true)
	configFlags.AddFlags(cmd.PersistentFlags())
	cmd.Flags().StringVar(&name, "name", "", "name of the Retina Capture")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Namespace to host capture job")
	return cmd
}
