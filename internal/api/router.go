package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"opennhrp-manager/internal/config"
	"opennhrp-manager/internal/db"
	"opennhrp-manager/internal/service"
)

func SetupRouter(
	cfg *config.Config,
	database *db.DB,
	nodeMgr *service.NodeManager,
	witnessSvc *service.WitnessService,
	provService *service.ProvisioningService,
	logHub *service.LogHub,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// Enable CORS for local Vite dev server
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	authHandler := NewAuthHandler(database, cfg.JWTSecret)
	userHandler := NewUserHandler(database)
	clusterHandler := NewClusterHandler(nodeMgr, database)
	spokeHandler := NewSpokeHandler(nodeMgr, database, provService)
	witnessHandler := NewWitnessHandler(witnessSvc, database)
	configHandler := NewConfigHandler(nodeMgr, database)
	managedSpokeHandler := NewManagedSpokeHandler(nodeMgr, database)
	agentWSHandler := NewAgentWSHandler(cfg, database, nodeMgr, logHub)
	logWSHandler := NewLogWSHandler(logHub)
	topologyWSHandler := NewTopologyWSHandler(nodeMgr, witnessSvc)

	authRequired := AuthRequired(cfg.JWTSecret)
	adminOnly := AdminOnly()

	apiGroup := r.Group("/api")
	{
		// Public Auth Endpoints
		apiGroup.POST("/auth/login", authHandler.Login)

		// Public Agent WebSocket connection (internal agent token checked inside handler)
		apiGroup.GET("/agent/ws", agentWSHandler.HandleWS)

		// Authenticated Routes
		protected := apiGroup.Group("", authRequired)
		{
			// Auth info & Self Password Change
			protected.GET("/auth/me", authHandler.GetMe)
			protected.POST("/auth/logout", authHandler.Logout)
			protected.PUT("/auth/change-password", authHandler.ChangePassword)

			// User Management (Admin Only)
			usersGroup := protected.Group("/users", adminOnly)
			{
				usersGroup.GET("", userHandler.ListUsers)
				usersGroup.POST("", userHandler.CreateUser)
				usersGroup.PUT("/:id", userHandler.UpdateUser)
				usersGroup.DELETE("/:id", userHandler.DeleteUser)
			}

			// Cluster & Hub HA
			clusterGroup := protected.Group("/cluster")
			{
				clusterGroup.GET("/status", clusterHandler.GetClusterStatus)
				clusterGroup.GET("/replication", clusterHandler.GetReplicationStatus)
				clusterGroup.GET("/members", clusterHandler.GetMembers)
				clusterGroup.GET("/invites", clusterHandler.ListInvites)
				clusterGroup.GET("/keys", clusterHandler.GetKeyStatus)
				clusterGroup.GET("/key/export-spoke", clusterHandler.ExportSpokeKey)

				// Admin-only mutating operations
				clusterGroup.POST("/member", adminOnly, clusterHandler.SetMember)
				clusterGroup.POST("/invite", adminOnly, clusterHandler.CreateInvite)
				clusterGroup.DELETE("/invite/:id_prefix", adminOnly, clusterHandler.DeleteInvite)
				clusterGroup.POST("/invite/:id_prefix/revoke", adminOnly, clusterHandler.RevokeInvite)
				clusterGroup.POST("/invite/:id_prefix/delete", adminOnly, clusterHandler.DeleteInvite)
				clusterGroup.POST("/join", adminOnly, clusterHandler.JoinCluster)
				clusterGroup.POST("/key/rotate", adminOnly, clusterHandler.RotateKey)
				clusterGroup.POST("/failback", adminOnly, clusterHandler.RequestFailback)
			}

			// Spokes
			spokeGroup := protected.Group("/spokes")
			{
				spokeGroup.GET("", spokeHandler.ListSpokes)

				// Admin-only mutating operations
				spokeGroup.POST("/map", adminOnly, spokeHandler.AddStaticMap)
				spokeGroup.DELETE("/map", adminOnly, spokeHandler.DelStaticMap)
				spokeGroup.POST("/map/save", adminOnly, spokeHandler.SaveMap)
				spokeGroup.POST("/nbma/update", adminOnly, spokeHandler.UpdateNBMA)
				spokeGroup.POST("/redirect/purge", adminOnly, spokeHandler.PurgeRedirect)
				spokeGroup.POST("/metadata", adminOnly, spokeHandler.SetSpokeMetadata)
				spokeGroup.POST("/provision/generate", adminOnly, spokeHandler.GenerateSpokeConfig)
				spokeGroup.POST("/provision/download", adminOnly, spokeHandler.DownloadSpokePackage)
			}

			managedSpokeGroup := protected.Group("/managed-spokes")
			{
				managedSpokeGroup.GET("", managedSpokeHandler.List)
				managedSpokeGroup.GET("/:id/peers", managedSpokeHandler.Peers)
				managedSpokeGroup.POST("", adminOnly, managedSpokeHandler.Create)
				managedSpokeGroup.POST("/:id/token/rotate", adminOnly, managedSpokeHandler.RotateToken)
				managedSpokeGroup.DELETE("/:id", adminOnly, managedSpokeHandler.Delete)
			}

			// Witness & SLA
			witnessGroup := protected.Group("/witness")
			{
				witnessGroup.GET("/quorum", witnessHandler.GetQuorum)
				witnessGroup.GET("/sla", witnessHandler.GetSLAMatrix)
				witnessGroup.GET("/probes", witnessHandler.GetRecentProbes)
				witnessGroup.GET("/arbitrations", witnessHandler.GetArbitrations)
			}

			// Configuration & System
			configGroup := protected.Group("/config")
			{
				configGroup.GET("/nodes", configHandler.ListNodes)
				configGroup.GET("/interfaces", configHandler.ListInterfaces)
				configGroup.GET("/file", configHandler.GetConfigFile)
				configGroup.GET("/audit-logs", configHandler.GetAuditLogs)

				// Admin-only mutating operations
				configGroup.POST("/file", adminOnly, configHandler.SaveConfigFile)
				configGroup.POST("/reload", adminOnly, configHandler.ReloadConfig)
			}

			// WebSocket Logs
			protected.GET("/logs/ws", logWSHandler.HandleWS)
			protected.GET("/topology/ws", topologyWSHandler.HandleWS)
		}
	}

	return r
}
