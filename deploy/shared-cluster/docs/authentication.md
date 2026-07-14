## Authentication through Keycloak and Authorization through OpenFGA

Authentication and authorization are two of the main concepts which would connect Naira (and its UI) with Platform Mesh. Naira will employ the hierarchical structure in Platform Mesh which would let it understand which user is from which `organization` or from which `account`. 

## Prerequisites

A running Platform Mesh cluster. This could be a local Platform Mesh cluster (you can create a Platform Mesh cluster with kind through [this link](https://platform-mesh.io/main/how-to-guides/set-up-platform-mesh-locally)) or the shared cluster which we are using.

## Setup

After creating the Platform Mesh cluster or using the shared cluster, you first need to register or log in with your credentials. In the screen that welcomes you, you first need to create (onboard) an organization under your username. You can create any `organization` name (preferred would be `naira`). After onboarding this organization, you can switch to that organization and do what is prompted on the redirected Keycloak screen. It will ask you to change the password and make you provide a bit more information.

Once you are in the workspace, our job with Platform Mesh UI is actually done. Don't forget to provide this organization's name under `Taskfile.yml` file under /deploy/dev for the variable `KEYCLOAK_REALM`. Otherwise your tokens would not be able to get validated by the backend correctly!

Next, we port-forward the Keycloak instance running inside the Platform Mesh cluster: `kubectl port-forward svc/keycloak -n platform-mesh-system 8080:80`. Open `http://localhost:8080` and you would be prompted to give the admin username and password.

Once you are inside the Keycloak admin realm, click on `Manage realms` at the top left. There, you will see the `master` realm and whatever organization you have created before in Platform Mesh. That is because organization names are mapped 1:1 to a Keycloak realm to store the clients, users inside this organization. Click on the name of the realm that you created to get the client. Once you are inside your realm, go to `Clients` and provide the Client ID whose `Display Name` is actually your realm's name.

Click on that Client and copy its Client ID. Put that Client ID under `OIDC_CLIENT_ID_KEYCLOAK` in `portal.yaml` under /deploy/dev/infra/kubernetes directory. Next, go to the `Credentials` tab and copy the Client Secret and paste it to `OIDC_CLIENT_SECRET_KEYCLOAK` Secret inside `portal.yaml`.

Go over to `Realm roles` at the left side and create two roles: `ai-engineer` and `application-engineer`. These will enable us to filter OpenFGA tuples based on roles in Keycloak. Go to your user under `Users` and select one of them (or both if you would like). Finally, go to your `CLient Scopes` and find the `roles` scope in the second page (you need to go to the next page under bottom right). Once you click on `roles` scope find `Mappers > realm roles > Toggle Add to ID token`. This is because OpenMFP receives both ID and access token from Keycloak, however it drops access token and instead returns ID token to the child components. If we would not toggle this, then the realm roles (and hence the differentiation of roles inherited from Keycloak such as `ai-engineer`) would never be read by the codebase.

After this, close the Keycloak and start the platform by doing `task platform:deploy`.

Now, do `task forward:all` and look at the `http://localhost:3000`. If you click on the blank avatar at the upper right corner, you would also see a `Sign out` option. If you sign out and sign back in, it would still be on the same session but if you sign out from your respective organization inside Platform Mesh before and you refresh you http://localhost:3000 page afterwards, then you will be redirected to your organization's Keycloak log in page in which you need to give the credentials that you gave while creating this respective organization. 

Now, you would be able to see only the options that your role is capable of seeing. Until now, `Models, Applications and Datasets` could be retrieved with the role `ai-engineer` whereas `Deployments and Services` could be retrieved with the role `application-engineer`.