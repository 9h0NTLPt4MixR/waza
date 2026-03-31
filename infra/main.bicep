// Waza Platform — Azure infrastructure
// Deploys: Container Apps + Cosmos DB + Key Vault + Managed Identity

targetScope = 'resourceGroup'

// ============================================================================
// Parameters
// ============================================================================

@description('Primary location for all resources')
param location string = resourceGroup().location

@description('Unique environment name (used as resource suffix)')
@minLength(1)
@maxLength(32)
param environmentName string

@description('GitHub OAuth App client ID')
@secure()
param githubClientId string

@description('GitHub OAuth App client secret')
@secure()
param githubClientSecret string

@description('JWT signing secret for session tokens')
@secure()
param jwtSecret string

@description('ADC API key (optional, for sandbox execution)')
@secure()
param adcApiKey string = ''

@description('Base64-encoded 32-byte AES-256 encryption key for connection configs')
@secure()
param encryptionKey string

@description('Container image for the platform (e.g., myregistry.azurecr.io/waza-platform:latest)')
param containerImage string = 'mcr.microsoft.com/k8se/quickstart:latest'

@description('Custom domain for the platform (optional)')
param customDomain string = ''

// ============================================================================
// Variables
// ============================================================================

var resourceToken = toLower(uniqueString(resourceGroup().id, environmentName))
var abbrs = {
  containerAppsEnvironment: 'cae-'
  containerApp: 'ca-'
  containerRegistry: 'cr'
  cosmosAccount: 'cosmos-'
  keyVault: 'kv-'
  managedIdentity: 'id-'
  logAnalytics: 'log-'
}

// ============================================================================
// Log Analytics Workspace
// ============================================================================

resource logAnalytics 'Microsoft.OperationalInsights/workspaces@2023-09-01' = {
  name: '${abbrs.logAnalytics}${resourceToken}'
  location: location
  properties: {
    sku: {
      name: 'PerGB2018'
    }
    retentionInDays: 30
  }
}

// ============================================================================
// Managed Identity
// ============================================================================

resource managedIdentity 'Microsoft.ManagedIdentity/userAssignedIdentities@2023-01-31' = {
  name: '${abbrs.managedIdentity}${resourceToken}'
  location: location
}

// ============================================================================
// Key Vault
// ============================================================================

// ============================================================================
// Azure Container Registry
// ============================================================================

resource containerRegistry 'Microsoft.ContainerRegistry/registries@2023-07-01' = {
  name: '${abbrs.containerRegistry}${resourceToken}'
  location: location
  sku: {
    name: 'Basic'
  }
  properties: {
    adminUserEnabled: false
  }
}

// AcrPull role for the managed identity
resource acrPullRole 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  scope: containerRegistry
  name: guid(containerRegistry.id, managedIdentity.id, '7f951dda-4ed3-4680-a7ca-43fe172d538d')
  properties: {
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', '7f951dda-4ed3-4680-a7ca-43fe172d538d')
    principalId: managedIdentity.properties.principalId
    principalType: 'ServicePrincipal'
  }
}

// ============================================================================

resource keyVault 'Microsoft.KeyVault/vaults@2023-07-01' = {
  name: take('${abbrs.keyVault}${resourceToken}', 24)
  location: location
  properties: {
    sku: {
      family: 'A'
      name: 'standard'
    }
    tenantId: subscription().tenantId
    enableRbacAuthorization: true
    enableSoftDelete: true
    softDeleteRetentionInDays: 7
  }
}

// Key Vault Secrets Officer role for the managed identity
resource kvSecretsRole 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  scope: keyVault
  name: guid(keyVault.id, managedIdentity.id, '4633458b-17de-408a-b874-0445c86b69e6')
  properties: {
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', '4633458b-17de-408a-b874-0445c86b69e6')
    principalId: managedIdentity.properties.principalId
    principalType: 'ServicePrincipal'
  }
}

// Store secrets in Key Vault
resource secretGithubClientId 'Microsoft.KeyVault/vaults/secrets@2023-07-01' = {
  parent: keyVault
  name: 'github-client-id'
  properties: {
    value: githubClientId
  }
}

resource secretGithubClientSecret 'Microsoft.KeyVault/vaults/secrets@2023-07-01' = {
  parent: keyVault
  name: 'github-client-secret'
  properties: {
    value: githubClientSecret
  }
}

resource secretJwtSecret 'Microsoft.KeyVault/vaults/secrets@2023-07-01' = {
  parent: keyVault
  name: 'jwt-secret'
  properties: {
    value: jwtSecret
  }
}

resource secretEncryptionKey 'Microsoft.KeyVault/vaults/secrets@2023-07-01' = {
  parent: keyVault
  name: 'encryption-key'
  properties: {
    value: encryptionKey
  }
}

resource secretAdcApiKey 'Microsoft.KeyVault/vaults/secrets@2023-07-01' = {
  parent: keyVault
  name: 'adc-api-key'
  properties: {
    value: adcApiKey
  }
}

// ============================================================================
// Cosmos DB
// ============================================================================

resource cosmosAccount 'Microsoft.DocumentDB/databaseAccounts@2024-05-15' = {
  name: '${abbrs.cosmosAccount}${resourceToken}'
  location: location
  kind: 'GlobalDocumentDB'
  properties: {
    databaseAccountOfferType: 'Standard'
    disableLocalAuth: true
    locations: [
      {
        locationName: location
        failoverPriority: 0
      }
    ]
    capabilities: [
      {
        name: 'EnableServerless'
      }
    ]
    consistencyPolicy: {
      defaultConsistencyLevel: 'Session'
    }
  }
}

resource cosmosDatabase 'Microsoft.DocumentDB/databaseAccounts/sqlDatabases@2024-05-15' = {
  parent: cosmosAccount
  name: 'waza-platform'
  properties: {
    resource: {
      id: 'waza-platform'
    }
  }
}

resource containerUsers 'Microsoft.DocumentDB/databaseAccounts/sqlDatabases/containers@2024-05-15' = {
  parent: cosmosDatabase
  name: 'users'
  properties: {
    resource: {
      id: 'users'
      partitionKey: {
        paths: ['/github_id']
        kind: 'Hash'
      }
    }
  }
}

resource containerConnections 'Microsoft.DocumentDB/databaseAccounts/sqlDatabases/containers@2024-05-15' = {
  parent: cosmosDatabase
  name: 'connections'
  properties: {
    resource: {
      id: 'connections'
      partitionKey: {
        paths: ['/user_id']
        kind: 'Hash'
      }
    }
  }
}

resource containerRunRequests 'Microsoft.DocumentDB/databaseAccounts/sqlDatabases/containers@2024-05-15' = {
  parent: cosmosDatabase
  name: 'run-requests'
  properties: {
    resource: {
      id: 'run-requests'
      partitionKey: {
        paths: ['/user_id']
        kind: 'Hash'
      }
    }
  }
}

resource containerSettings 'Microsoft.DocumentDB/databaseAccounts/sqlDatabases/containers@2024-05-15' = {
  parent: cosmosDatabase
  name: 'settings'
  properties: {
    resource: {
      id: 'settings'
      partitionKey: {
        paths: ['/id']
        kind: 'Hash'
      }
    }
  }
}

resource containerResults 'Microsoft.DocumentDB/databaseAccounts/sqlDatabases/containers@2024-05-15' = {
  parent: cosmosDatabase
  name: 'results'
  properties: {
    resource: {
      id: 'results'
      partitionKey: {
        paths: ['/user_id']
        kind: 'Hash'
      }
    }
  }
}

// Cosmos DB Built-in Data Contributor role for the managed identity
resource cosmosDataContributorRole 'Microsoft.DocumentDB/databaseAccounts/sqlRoleAssignments@2024-05-15' = {
  parent: cosmosAccount
  name: guid(cosmosAccount.id, managedIdentity.id, 'cosmos-data-contributor')
  properties: {
    roleDefinitionId: '${cosmosAccount.id}/sqlRoleDefinitions/00000000-0000-0000-0000-000000000002'
    principalId: managedIdentity.properties.principalId
    scope: cosmosAccount.id
  }
}

// ============================================================================
// Container Apps Environment
// ============================================================================

resource containerAppsEnv 'Microsoft.App/managedEnvironments@2024-03-01' = {
  name: '${abbrs.containerAppsEnvironment}${resourceToken}'
  location: location
  properties: {
    appLogsConfiguration: {
      destination: 'log-analytics'
      logAnalyticsConfiguration: {
        customerId: logAnalytics.properties.customerId
        sharedKey: logAnalytics.listKeys().primarySharedKey
      }
    }
  }
}

// ============================================================================
// Container App — waza-platform
// ============================================================================

resource containerApp 'Microsoft.App/containerApps@2024-03-01' = {
  name: '${abbrs.containerApp}${resourceToken}'
  location: location
  tags: {
    'azd-service-name': 'platform'
  }
  identity: {
    type: 'UserAssigned'
    userAssignedIdentities: {
      '${managedIdentity.id}': {}
    }
  }
  properties: {
    managedEnvironmentId: containerAppsEnv.id
    configuration: {
      registries: [
        {
          server: containerRegistry.properties.loginServer
          identity: managedIdentity.id
        }
      ]
      ingress: {
        external: true
        targetPort: 3000
        transport: 'http'
        corsPolicy: {
          allowedOrigins: customDomain != '' ? ['https://${customDomain}'] : ['*']
          allowedMethods: ['GET', 'POST', 'PUT', 'DELETE', 'OPTIONS']
          allowedHeaders: ['*']
        }
      }
      secrets: [
        {
          name: 'github-client-id'
          keyVaultUrl: secretGithubClientId.properties.secretUri
          identity: managedIdentity.id
        }
        {
          name: 'github-client-secret'
          keyVaultUrl: secretGithubClientSecret.properties.secretUri
          identity: managedIdentity.id
        }
        {
          name: 'jwt-secret'
          keyVaultUrl: secretJwtSecret.properties.secretUri
          identity: managedIdentity.id
        }
        {
          name: 'encryption-key'
          keyVaultUrl: secretEncryptionKey.properties.secretUri
          identity: managedIdentity.id
        }
      ]
    }
    template: {
      containers: [
        {
          name: 'waza-platform'
          image: containerImage
          resources: {
            cpu: json('0.5')
            memory: '1Gi'
          }
          env: [
            { name: 'COSMOS_ENDPOINT', value: cosmosAccount.properties.documentEndpoint }
            { name: 'AZURE_CLIENT_ID', value: managedIdentity.properties.clientId }
            { name: 'ENCRYPTION_KEY', secretRef: 'encryption-key' }
            { name: 'GITHUB_CLIENT_ID', secretRef: 'github-client-id' }
            { name: 'GITHUB_CLIENT_SECRET', secretRef: 'github-client-secret' }
            { name: 'GITHUB_REDIRECT_URL', value: customDomain != '' ? 'https://${customDomain}/api/auth/callback' : 'https://${abbrs.containerApp}${resourceToken}.${containerAppsEnv.properties.defaultDomain}/api/auth/callback' }
            { name: 'JWT_SECRET', secretRef: 'jwt-secret' }
            { name: 'ADC_API_KEY', value: adcApiKey }
          ]
          probes: [
            {
              type: 'Liveness'
              httpGet: {
                path: '/healthz'
                port: 3000
              }
              periodSeconds: 30
            }
            {
              type: 'Readiness'
              httpGet: {
                path: '/healthz'
                port: 3000
              }
              initialDelaySeconds: 5
              periodSeconds: 10
            }
          ]
        }
      ]
      scale: {
        minReplicas: 1
        maxReplicas: 3
        rules: [
          {
            name: 'http-scaling'
            http: {
              metadata: {
                concurrentRequests: '50'
              }
            }
          }
        ]
      }
    }
  }
}

// ============================================================================
// Outputs
// ============================================================================

output AZURE_CONTAINER_APP_FQDN string = containerApp.properties.configuration.ingress.fqdn
output AZURE_COSMOS_ENDPOINT string = cosmosAccount.properties.documentEndpoint
output AZURE_KEY_VAULT_NAME string = keyVault.name
output AZURE_MANAGED_IDENTITY_CLIENT_ID string = managedIdentity.properties.clientId
output AZURE_CONTAINER_REGISTRY_ENDPOINT string = containerRegistry.properties.loginServer
output AZURE_CONTAINER_REGISTRY_NAME string = containerRegistry.name
