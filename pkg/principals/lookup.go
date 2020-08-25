package principals

import (
	"fmt"
	"strings"
	"sync"
	"time"

	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"

	corev1 "k8s.io/api/core/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"

	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"

	"github.com/rancher/rancher-operator/pkg/clients"

	"k8s.io/client-go/tools/cache"

	"github.com/rancher/norman/clientbase"
	mgmtcontrollers "github.com/rancher/rancher-operator/pkg/generated/controllers/management.cattle.io/v3"
	"github.com/rancher/rancher-operator/pkg/settings"
	client "github.com/rancher/rancher/pkg/client/generated/management/v3"
)

const (
	byBootstrap    = "by-bootstrapping"
	byUser         = "by-user"
	bootstrapLabel = "authz.management.cattle.io/bootstrapping"
	adminUser      = "admin-user"
)

type Lookup struct {
	sync.Mutex

	cache           cache.Store
	userCache       mgmtcontrollers.UserCache
	tokenCache      mgmtcontrollers.TokenCache
	secret          corecontrollers.SecretCache
	secretName      string
	secretNamespace string

	serverURL, cacert string
	settings          mgmtcontrollers.SettingCache
	client            *client.Client
	collection        *client.PrincipalCollection
}

type entry struct {
	key   string
	value string
}

func NewLookup(secretNamespace, secretName string, clients *clients.Clients) *Lookup {
	clients.Management.User().Cache().AddIndexer(byBootstrap, func(obj *v3.User) ([]string, error) {
		val := obj.Labels[bootstrapLabel]
		if val != "" {
			return []string{val}, nil
		}
		return nil, nil
	})
	clients.Management.Token().Cache().AddIndexer(byUser, func(obj *v3.Token) ([]string, error) {
		return []string{obj.UserID}, nil
	})

	return &Lookup{
		cache: cache.NewTTLStore(func(obj interface{}) (string, error) {
			return obj.(entry).key, nil
		}, 5*time.Minute),
		userCache:       clients.Management.User().Cache(),
		tokenCache:      clients.Management.Token().Cache(),
		secret:          clients.Core.Secret().Cache(),
		secretName:      secretName,
		secretNamespace: secretNamespace,
		settings:        clients.Management.Setting().Cache(),
	}
}

func (l *Lookup) LookupUser(name string) (string, error) {
	return l.lookupPrincipal(name, "user")
}

func (l *Lookup) LookupGroup(name string) (string, error) {
	return l.lookupPrincipal(name, "group")
}

func (l *Lookup) lookupPrincipal(name, principleType string) (string, error) {
	cacheKey := fmt.Sprintf("%s/%s", principleType, name)

	val, ok, err := l.cache.GetByKey(cacheKey)
	if err != nil {
		return "", err
	} else if ok {
		return val.(entry).value, nil
	}

	c, err := l.getClient()
	if err != nil {
		return "", err
	}

	col, err := c.Principal.CollectionActionSearch(l.collection, &client.SearchPrincipalsInput{
		Name:          name,
		PrincipalType: principleType,
	})
	if err != nil {
		return "", err
	}

	for _, col := range col.Data {
		if strings.EqualFold(col.Name, name) && col.PrincipalType == principleType {
			return col.ID, l.cache.Add(entry{key: cacheKey, value: col.ID})
		}
	}

	return "", fmt.Errorf("principle not found for %s %s", principleType, name)
}

func (l *Lookup) getClient() (*client.Client, error) {
	l.Lock()
	defer l.Unlock()

	serverURL, cacert, err := settings.GetServerURLAndCA(l.settings)
	if err != nil {
		return nil, err
	}

	if l.serverURL == serverURL && l.cacert == cacert {
		return l.client, nil
	}

	accessKey, secretKey, err := l.getAccessKeyAndSecretKey()
	if err != nil {
		return nil, err
	}

	c, err := client.NewClient(&clientbase.ClientOpts{
		URL:       serverURL + "/v3",
		AccessKey: accessKey,
		SecretKey: secretKey,
		CACerts:   cacert,
	})
	if err != nil {
		return nil, err
	}

	collection, err := c.Principal.List(nil)
	if err != nil {
		return nil, err
	}

	l.client = c
	l.collection = collection
	return c, nil
}

func (l *Lookup) getAccessKeyAndSecretKey() (string, string, error) {
	if l.secretName != "" {
		secret, err := l.secret.Get(l.secretNamespace, l.secretName)
		if err != nil && !apierror.IsNotFound(err) {
			return "", "", err
		} else if !apierror.IsNotFound(err) {
			return string(secret.Data[corev1.BasicAuthUsernameKey]), string(secret.Data[corev1.BasicAuthPasswordKey]), nil
		}
	}

	users, err := l.userCache.GetByIndex(byBootstrap, adminUser)
	if err != nil {
		return "", "", err
	}
	if len(users) == 0 {
		return "", "", fmt.Errorf("failed to find bootstrap admin user")
	}

	for _, user := range users {
		tokens, err := l.tokenCache.GetByIndex(byUser, user.Name)
		if apierror.IsNotFound(err) {
			continue
		} else if err != nil {
			return "", "", err
		}
		for _, token := range tokens {
			if token.Token != "" && !token.IsDerived && !token.Expired && (token.Enabled == nil || *token.Enabled) {
				return token.Name, token.Token, nil
			}
		}
	}

	return "", "", fmt.Errorf("failed to find token for bootstrap admin user")
}
