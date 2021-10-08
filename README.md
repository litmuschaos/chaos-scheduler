# Litmus chaos-scheduler for injecting chaos experiments on Kubernetes

[![Slack Channel](https://img.shields.io/badge/Slack-Join-purple)](https://slack.litmuschaos.io)
![GitHub Workflow](https://github.com/litmuschaos/chaos-scheduler/actions/workflows/push.yml/badge.svg?branch=master)
[![Docker Pulls](https://img.shields.io/docker/pulls/litmuschaos/chaos-scheduler.svg)](https://hub.docker.com/r/litmuschaos/chaos-scheduler)
[![GitHub issues](https://img.shields.io/github/issues/litmuschaos/chaos-scheduler)](https://github.com/litmuschaos/chaos-scheduler/issues)
[![Twitter Follow](https://img.shields.io/twitter/follow/litmuschaos?style=social)](https://twitter.com/LitmusChaos)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/5299/badge)](https://bestpractices.coreinfrastructure.org/projects/5299)
[![Go Report Card](https://goreportcard.com/badge/github.com/litmuschaos/chaos-scheduler)](https://goreportcard.com/report/github.com/litmuschaos/chaos-scheduler)
[![YouTube Channel](https://img.shields.io/badge/YouTube-Subscribe-red)](https://www.youtube.com/channel/UCa57PMqmz_j0wnteRa9nCaw)
  
Litmus chaos scheduler is used by Kubernetes application developers and SREs to inject chaos into the applications 
and Kubernetes infrastructure periodically based on the specified schedule. Perform the following steps to use the chaos scheduler: 

- Install the Litmus infrastructure components (RBAC, CRDs), the scheduler, the operator & Experiment custom resource bundles via helm charts
- Create a ChaosSchedule custom resource, which describes the ChaosEngine template to be scheduled 

## What is a chaos scheduler and how is it built?

The Chaos scheduler is a custom-controllers with direct access to Kubernetes API that can manage the lifecycle of certain resources or applications, 
while always trying to ensure the resource is in the "desired state". The logic that ensures this is commonly called "reconcile" function.

The Chaos scheduler is built using [Operator-SDK](https://github.com/operator-framework/operator-sdk/) framework, 
which provides bootstrap support for new scheduler projects, allowing teams to focus on business/operational logic. 

## What is a chaos schedule?

Currently, it defines the following:

- Execution Schedule for the batch run of the experiments
- Template Spec of ChaosEngine according to which chaos is to be exceuted

The ChaosSchedule is referenced as the owner of the secondary (reconcile) resource with Kubernetes deletePropagation 
ensuring these also are removed upon deletion of the ChaosSchedule CR.

### Sample ChaosSchedule for reference:

```yaml
apiVersion: litmuschaos.io/v1alpha1
kind: ChaosSchedule
metadata:
  name: schedule-nginx
spec:
  schedule:
    repeat:
      timeRange:
        startTime: "2020-05-12T05:47:00Z"   #should be modified according to current UTC Time, for type=repeat
        endTime: "2020-09-13T02:58:00Z"   #should be modified according to current UTC Time, for type=repeat
      properties:
        minChaosInterval: "2m"   #format should be like "10m" or "2h" accordingly for minutes and hours, for type=repeat
      workHours:
        includedHours: 0-12
      workDays:
        includedDays: "Mon,Tue,Wed,Sat,Sun" #should be set for type=repeat
  engineTemplateSpec:
    appinfo:
      appns: 'default'
      applabel: 'app=nginx'
      appkind: 'deployment'
    # It can be true/false
    annotationCheck: 'false'
    # It can be active/stop
    engineState: 'active'
    #ex. values: ns1:name=percona,ns2:run=nginx
    auxiliaryAppInfo: ''
    chaosServiceAccount: pod-delete-sa
    # It can be delete/retain
    jobCleanUpPolicy: 'delete'
    experiments:
      - name: pod-delete
        spec:
          components:
            env:
              # set chaos duration (in sec) as desired
              - name: TOTAL_CHAOS_DURATION
                value: '30'

              # set chaos interval (in sec) as desired
              - name: CHAOS_INTERVAL
                value: '10'

              # pod failures without '--force' & default terminationGracePeriodSeconds
              - name: FORCE
                value: 'false'
```

## What is a chaos engine?

The ChaosEngine is the main user-facing chaos custom resource with a namespace scope and is designed to hold information around how the chaos experiments are executed. It connects an application instance with one or more chaos experiments,

For more details [refer](https://v1-docs.litmuschaos.io/docs/chaosengine/) 

## What is a litmus chaos chart and how can I use it?

Litmus chaos chart is the heart of litmus and contains the low-level execution information. They serve as off-the-shelf templates that one needs to "pull" (install on the cluster) before including them as part of a chaos run against any target applications (the binding being defined in the ChaosEngine).

For more details refer [litmus docs](https://v1-docs.litmuschaos.io/docs/chaosexperiment/) and [hub](https://hub.litmuschaos.io/).

## What are the steps to get started?

- Install Operator Components and RBAC and CRDs

  ```bash
  kubectl apply -f https://litmuschaos.github.io/litmus/litmus-operator-latest.yaml
  ```

- Install Scheduler and it's CRDs

  ```bash
  kubectl apply -f https://raw.githubusercontent.com/litmuschaos/chaos-scheduler/master/deploy/crds/chaosschedule_crd.yaml
  
  kubectl apply -f https://raw.githubusercontent.com/litmuschaos/chaos-scheduler/master/deploy/chaos-scheduler.yaml
  ```   

- Create the pod delete Chaos Experiment in default namespace

  ```
  kubectl apply -f https://raw.githubusercontent.com/litmuschaos/chaos-charts/master/charts/generic/pod-delete/experiment.yaml
  ```

- Create the RBAC for execute the pod-delete chaos

  ```bash
  kubectl apply -f https://raw.githubusercontent.com/litmuschaos/chaos-charts/master/charts/generic/pod-delete/rbac.yaml
  ```

- Annotate your application to enable chaos. For ex:

  ```
  kubectl annotate deploy/nginx-deployment litmuschaos.io/chaos="true"
  ```

- Create a ChaosSchedule yaml with the application information and chaos experiment with their scheduling logic, For example: [Click Here](#Sample-ChaosSchedule-for-reference)

- Create a ChaosSchedule customer resource in the desired cluster, For example

  ```bash
  kubectl apply -f chaos-schedule.yaml
  ```

- Watch the injection of chaos at the scheduled time, For example
  ```bash
  watch kubectl get pod,chaosschedule,chaosengine -n litmus
  ```

- Describe the ChaosSchedule for see the detail of chaos injection.
  ```bash
  kubectl describe chaosschedule schedule-nginx
  ```

  Refer the ChaosSchedule Status. While any ChaosEngine is active or yet to be formed according to the schedule time. The `.status.schedule.status` is set to `running` eventually changed to `completed`


  ```yaml
  Name:         schedule-nginx
  Namespace:    default
  Labels:       <none>
  Annotations:  API Version:  litmuschaos.io/v1alpha1
  Kind:         ChaosSchedule
  Metadata:
    Creation Timestamp:  2020-05-14T08:44:32Z
    Generation:          3
    Resource Version:    899464
    Self Link:           /apis/litmuschaos.io/v1alpha1/namespaces/default/chaosschedules/  schedule-nginx
    UID:                 347fb7e6-2c9d-428e-9ce1-42bdcfdab37d
  Spec:
    Chaos Service Account:  
    Engine Template Spec:
      Appinfo:
        Appkind:              deployment
        Applabel:             app=nginx
        Appns:                default
      Chaos Service Account:  litmus
      Components:
        Runner:
      Experiments:
        Name:  pod-delete
        Spec:
          Components:
          Rank:             0
      Job Clean Up Policy:  retain
    Schedule:
      Repeat:
        End Time:            2020-05-12T05:52:00Z
        Included Days:       Mon,Tue,Wed
        Instance Count:      2
        Min Chaos Interval:  2m
        Start Time:          2020-05-12T05:47:00Z
    Schedule State:        active
  Status:
    Active:
      API Version:       litmuschaos.io/v1alpha1
      Kind:              ChaosEngine
      Name:              schedule-nginx
      Namespace:         default
      Resource Version:  899463
      UID:               14f49857-8879-4129-a5b9-a3a592149725
    Last Schedule Time:  2020-05-14T08:44:32Z
    Schedule:
      Start Time:              2020-05-14T08:44:32Z
      Status:                  running
      Total Instances:         1
  Events:
    Type    Reason            Age   From             Message
    ----    ------            ----  ----             -------
    Normal  SuccessfulCreate  39s   chaos-scheduler  Created engine schedule-nginx
  ```


## How to halt the chaosschedule?

- Edit the applied chaosschedule

  ```bash
  kubectl edit chaosschedule schedule-nginx
  ```

- Change the state to `halt`

  ```yaml
  spec:
    scheduleState: halt
  ```

## How to resume the chaosschedule?

- Edit the applied chaosschedule

  ```bash
  kubectl edit chaosschedule schedule-nginx
  ```

- Change the state to `active`

  ```yaml
  spec:
    scheduleState: active
  ```

- If you face any probelem check the [Troubleshooting Guide](https://docs.litmuschaos.io/docs/faq-troubleshooting/)

## Where are the docs?

They are available at [litmus docs](https://docs.litmuschaos.io) and [experiment docs](https://litmuschaos.github.io/litmus/experiments/concepts/chaos-resources/contents/)

## How do I contribute?

You can contribute by raising issues, improving the documentation, contributing to the core framework and tooling, etc.

Head over to the [Contribution guide](CONTRIBUTING.md)
