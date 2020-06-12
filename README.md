
# Litmus chaos-scheduler for injecting chaos experiments on Kubernetes
  
Litmus chaos scheduler is used by Kubernetes application developers and SREs to inject chaos into the applications 
and Kubernetes infrastructure in a managed fashion. Its objective is to make the process of validation and 
hardening of application workloads on Kubernetes easy by automating the execution of chaos experiments. A sample chaos 
injection workflow could be as simple as:

- Install the Litmus infrastructure components (RBAC, CRDs), the scheduler, the operator & Experiment custom resource bundles via helm charts
- Annotate the application under test (AUT), enabling it for chaos
- Create a ChaosSchedule custom resource, which describes the ChaosEngine template to be scheduled 

## What is a chaos scheduler and how is it built?

The Chaos scheduler is a Kubernetes scheduler, which are nothing but custom-controllers with direct access to Kubernetes API
that can manage the lifecycle of certain resources or applications, while always trying to ensure the resource is in the "desired
state". The logic that ensures this is commonly called "reconcile" function.

The Chaos scheduler is built using the popular [Operator-SDK](https://github.com/operator-framework/operator-sdk/) framework, 
which provides bootstrap support for new scheduler projects, allowing teams to focus on business/operational logic. 

The Litmus Chaos scheduler helps reconcile the state of the ChaosSchedule, a custom resource that holds the chaos intent 
specified by a developer/devops engineer against a particular stateless/stateful Kubernetes deployment. The scheduler performs
specific actions upon CRUD of the ChaosSchedule, its primary resource. The scheduler also defines secondary resources (the 
ChaosEngine), which are created & managed by it in order to implement the reconcile functions. 

## What is a chaos schedule?

The ChaosSchedule is the core schema that defines the chaos workflow for a given application. Currently, it defines the following:

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
  namespace: litmus
spec:
  schedule:
    repeat:
      executionTime: "2020-05-11T20:30:00Z"   #should be set for type=once
      startTime: "2020-05-12T05:47:00Z"   #should be modified according to current UTC Time
      endTime: "2020-05-12T05:52:00Z"   #should be modified according to current UTC Time
      minChaosInterval: "2m"   #format should be like "10m" or "2h" accordingly for minute and hours
      instanceCount: "2"
      includedDays: "Mon,Tue,Wed"
  engineTemplateSpec:
    appinfo:
      appns: 'default'
      applabel: 'app=nginx'
      appkind: 'deployment'
    # It can be true/false
    annotationCheck: 'true'
    #ex. values: ns1:name=percona,ns2:run=nginx
    auxiliaryAppInfo: ''
    chaosServiceAccount: pod-delete-sa
    monitoring: false
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

Refer 
- https://github.com/litmuschaos/chaos-operator
- https://docs.litmuschaos.io/docs/getstarted/

## What is a litmus chaos chart and how can I use it?

Refer 
- https://github.com/litmuschaos/chaos-charts
- https://hub.litmuschaos.io/

## What are the steps to get started?

- Install Operator Components and RBAC and CRDs

  ```bash
  kubectl apply -f https://litmuschaos.github.io/pages/litmus-operator-latest.yaml
  ```

- Install Scheduler and it's CRDs

  ```bash
  kubectl apply -f https://raw.githubusercontent.com/litmuschaos/chaos-scheduler/master/deploy/crds/chaosschedule_crd.yaml
  
  kubectl apply -f https://raw.githubusercontent.com/litmuschaos/chaos-scheduler/master/deploy/chaos-scheduler.yaml
  ```   

- Create the pod delete Chaos Experiment in default namespace

  ```
  kubectl apply -f https://raw.githubusercontent.com/litmuschaos/chaos-charts/1.4.0/charts/generic/pod-delete/experiment.yaml
  ```

- Create the RBAC for execute the pod-delete chaos

  ```bash
  kubectl apply -f https://raw.githubusercontent.com/litmuschaos/chaos-charts/1.4.0/charts/generic/pod-delete/rbac.yaml
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

They are available at [litmus docs](https://docs.litmuschaos.io)

## How do I contribute?

The Chaos scheduler is in _alpha_ stage and needs all the help you can provide! Please contribute by raising issues, 
improving the documentation, contributing to the core framework and tooling, etc.
