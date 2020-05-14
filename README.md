
# Litmus chaos-scheduler for injecting chaos experiments on Kubernetes
  
Litmus chaos scheduler is used by Kubernetes application developers and SREs to inject chaos into the applications 
and Kubernetes infrastructure in a managed fashion. Its objective is to make the process of validation and 
hardening of application workloads on Kubernetes easy by automating the execution of chaos experiments. A sample chaos 
injection workflow could be as simple as:

- Install the Litmus infrastructure components (RBAC, CRDs), the scheduler, the operator & Experiment custom resource bundles via helm charts
- Annotate the application under test (AUT), enabling it for chaos
- Create a ChaosSchedule custom resource, which describes the ChaosEngine template to be scheduled 

Benefits provided by the Chaos scheduler include: 

- Scheduled batch Run of Chaos
- Standardised chaos experiment spec 
- Categorized chaos bundles for stateless/stateful/vendor-specific
- Test-Run resiliency 
- Ability to chaos run as a background service based on annotations

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

Here is a sample ChaosSchedule for reference: 

  ```yaml
  apiVersion: litmuschaos.io/v1alpha1
  kind: ChaosSchedule
  metadata:
    name: schedule-nginx
  spec:
    schedule:
      type: "now"
      executionTime: "2020-05-11T20:30:00Z"
      startTime: "2020-05-12T05:47:00Z"
      endTime: "2020-05-12T05:52:00Z"
      minChaosInterval: "2m"   #format should be like "10m" or "2h" accordingly for minutes and   hours
      instanceCount: "2"
      includedDays: 0-6
      random: false
    engineTemplateSpec:
      jobCleanUpPolicy: "retain"
      engineState: "active"
      auxiliaryAppInfo: ""
      appinfo:
        appns: default
        applabel: "app=nginx"
        appkind: deployment
      chaosServiceAccount: litmus
      monitoring: false
      experiments:
      - name: pod-delete 
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

- Install Litmus infrastructure (RBAC, CRD, operator, scheduler) components 

  ```
  helm repo add https://litmuschaos.github.io/chaos-charts
  helm repo update
  helm install litmuschaos/litmusInfra --namespace=litmus
  ```

- Install Scheduler 

  ```
  kubectl apply -f https://raw.githubusercontent.com/litmuschaos/chaos-scheduler/master/deploy/chaos-scheduler.yaml
  ```   

- Download the desired Chaos Experiment bundles, say, general Kubernetes chaos

  ```
  helm install litmuschaos/k8sChaos
  ```

- Annotate your application to enable chaos. For ex:

  ```
  kubectl annotate deploy/nginx-deployment litmuschaos.io/chaos="true"
  ```

- Create a ChaosEngine CR with application information & chaos experiment list with their respective attributes

  ```
  # engine-nginx.yaml is a chaosengine manifest file
  kubectl apply -f schedule-nginx.yaml
  ``` 

- Refer the ChaosSchedule Status. While any ChaosEngine is active or yet to be formed in the    future `.status.schedule.status` is set to running eventually changed to completed

  ```
  kubectl describe chaosschedule schedule-nginx
  
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
      Engine State:  active
      Experiments:
        Name:  pod-delete
        Spec:
          Components:
          Rank:             0
      Job Clean Up Policy:  retain
    Schedule:
      End Time:            2020-05-12T05:52:00Z
      Execution Time:      2020-05-11T20:30:00Z
      Included Days:       0-6
      Instance Count:      2
      Min Chaos Interval:  2m
      Random:              false
      Start Time:          2020-05-12T05:47:00Z
      Type:                now
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

## Where are the docs?

They are available at [litmus docs](https://docs.litmuschaos.io)

## How do I contribute?

The Chaos scheduler is in _alpha_ stage and needs all the help you can provide! Please contribute by raising issues, 
improving the documentation, contributing to the core framework and tooling, etc.
